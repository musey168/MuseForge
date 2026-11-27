package repo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agentflow/internal/domain"
)

func TestDatasourceKindTellsSQLiteAndMySQLApart(t *testing.T) {
	for _, tc := range []struct {
		dsn  string
		want string
	}{
		{"file:/data/museforge.db", datasourceSQLite},
		{"file::memory:?cache=shared", datasourceSQLite},
		{"/data/museforge.db", datasourceSQLite},
		{"data/museforge.db", datasourceSQLite},
		{"museforge.db", datasourceSQLite},
		{"loopforge:secret@tcp(127.0.0.1:3306)/loopforge", datasourceMySQL},
		{"tcp(127.0.0.1:3306)/loopforge", datasourceMySQL},
		{"mysql://loopforge@127.0.0.1:3306/loopforge", datasourceMySQL},
	} {
		if got := datasourceKind(tc.dsn); got != tc.want {
			t.Fatalf("datasourceKind(%q) = %q, want %q", tc.dsn, got, tc.want)
		}
	}
}

func TestOpenSQLiteCreatesSchemaFromModelManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "museforge.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if driver := s.StorageInfo()["driver"]; driver != datasourceSQLite {
		t.Fatalf("storage driver = %v, want %q", driver, datasourceSQLite)
	}
	for _, table := range []string{
		"issues", "issue_comments", "projects", "project_credentials",
		"users", "user_configs", "runners", "runner_tasks",
		"insight_sources", "insight_executions",
	} {
		if !s.DB.Migrator().HasTable(table) {
			t.Fatalf("fresh sqlite database is missing table %q", table)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sqlite file was not created: %v", err)
	}
}

func TestOpenSQLiteInMemory(t *testing.T) {
	for _, dsn := range []string{"file::memory:?cache=shared", "file:agentflow_open_test?mode=memory&cache=shared"} {
		s, err := Open(dsn)
		if err != nil {
			t.Fatalf("open %q: %v", dsn, err)
		}
		if driver := s.StorageInfo()["driver"]; driver != datasourceSQLite {
			t.Fatalf("storage driver = %v, want %q", driver, datasourceSQLite)
		}
		if !s.DB.Migrator().HasTable("issues") {
			t.Fatalf("in-memory sqlite database %q is missing the issues table", dsn)
		}
		s.Close()
	}
}

// The incremental DDL runs on SQLite too: a column that is missing from an
// existing file is added back on the next start.
func TestOpenSQLiteAddsMissingColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "museforge.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Exec("ALTER TABLE issues DROP COLUMN kind").Error; err != nil {
		s.Close()
		t.Skipf("this sqlite build cannot drop a column: %v", err)
	}
	s.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.DB.Migrator().HasColumn(&issueRow{}, "kind") {
		t.Fatal("the missing column was not added back")
	}
}

// A restart against the same file keeps what the platform recorded: the
// requirement, the project it belongs to, the personal credentials and the
// reported usage.
func TestSQLiteRestartPreservesRequirementsProjectsCredentialsAndUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "museforge.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PutUser(domain.User{StaffID: 7, LoginName: "muse", ChineseName: "缪斯"}); err != nil {
		t.Fatal(err)
	}
	user, ok := s.UserByLoginName("muse")
	if !ok {
		t.Fatal("user was not stored")
	}
	if err := s.PutProject(domain.Project{ID: "atlas", Key: "ATL", Name: "Atlas", OwnerID: user.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "req-1", ProjectID: "atlas", Title: "把用量台接到 SQLite", Description: "写入即持久", Status: "todo", Priority: "high", AssigneeType: "member", AssigneeUserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateUserCredentials(user.ID, UserCredentialUpdate{CodeBuddyAPIKey: "ck_secret"}); err != nil {
		t.Fatal(err)
	}
	source := domain.InsightSource{
		ID: "src-1", ProjectID: "atlas", Name: "runner",
		UserMappings: map[string]int64{"external-user": user.ID},
		TokenPrefix:  "muse", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := s.CreateInsightSource(source, "hash-1"); err != nil {
		t.Fatal(err)
	}
	execution := domain.InsightExecution{
		ID: "exec-1", Type: "workflow", ExternalExecutionID: "ext-1",
		Workflow: domain.InsightWorkflow{ID: "wf-1", Name: "用量上报"},
		ExternalUserID: "external-user", Status: "success",
		StartedAt: time.Now().UTC().Add(-time.Minute), FinishedAt: time.Now().UTC(),
		Usage: []domain.InsightUsage{{Provider: "omp", Model: "muse-1", InputTokens: 120, OutputTokens: 34, CostStatus: "unknown"}},
	}
	if duplicate, err := s.IngestInsightExecution(source, execution); err != nil || duplicate {
		t.Fatalf("ingest insight execution: duplicate=%v err=%v", duplicate, err)
	}
	s.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	issue, ok := reopened.Issue("req-1")
	if !ok || issue.Title != "把用量台接到 SQLite" || issue.ProjectID != "atlas" {
		t.Fatalf("requirement did not survive the restart: %+v", issue)
	}
	project, ok := reopened.Project("atlas")
	if !ok || project.Name != "Atlas" {
		t.Fatalf("project did not survive the restart: %+v", project)
	}
	stored, ok := reopened.User(user.ID)
	if !ok || stored.CodeBuddyAPIKey != "ck_secret" {
		t.Fatalf("credentials did not survive the restart: %+v", stored)
	}
	storedSource, err := reopened.InsightSource("src-1")
	if err != nil || storedSource.Name != "runner" {
		t.Fatalf("insight source did not survive the restart: %+v err=%v", storedSource, err)
	}
	rows, err := reopened.insightExecutions(domain.AnalyticsFilter{ProjectID: "atlas"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ContentHash == "" {
		t.Fatalf("reported usage did not survive the restart: %+v", rows)
	}
	var reported domain.InsightExecution
	if err := json.Unmarshal(rows[0].Payload, &reported); err != nil {
		t.Fatal(err)
	}
	if reported.ID != "exec-1" || len(reported.Usage) != 1 || reported.Usage[0].InputTokens != 120 || reported.Usage[0].OutputTokens != 34 {
		t.Fatalf("reported usage did not survive the restart: %+v", reported)
	}
}

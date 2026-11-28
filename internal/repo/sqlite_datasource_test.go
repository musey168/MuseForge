package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

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

// TestOpenSQLiteCreatesMissingParentDirectory covers both DSN spellings of a
// file-backed database: the bare path and the "file:" URI that
// scripts/configure.sh writes. Both must create the directory the file lives
// in, so a fresh install needs one file and nothing else.
func TestOpenSQLiteCreatesMissingParentDirectory(t *testing.T) {
	for _, dsn := range []string{
		"file:%s/museforge.db",
		"%s/museforge.db",
	} {
		dir := filepath.Join(t.TempDir(), "nested", "data")
		s, err := Open(fmt.Sprintf(dsn, dir))
		if err != nil {
			t.Fatalf("open %q: %v", fmt.Sprintf(dsn, dir), err)
		}
		s.Close()
		if _, err := os.Stat(filepath.Join(dir, "museforge.db")); err != nil {
			t.Fatalf("sqlite file was not created for %q: %v", fmt.Sprintf(dsn, dir), err)
		}
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
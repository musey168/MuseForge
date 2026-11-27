// Package repo is the gorm-backed persistence layer of AgentFlow. MySQL and
// SQLite are both production datasources; tests open an in-process sqlite
// database through the same gorm code paths. There is no second in-memory
// implementation.
package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"agentflow/internal/credcrypt"
	"agentflow/internal/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var (
	ErrRunDecisionStale  = errors.New("run changed before the decision was committed")
	ErrRunLeaseHeld      = errors.New("run lease is held by another instance")
	ErrRunLeaseLost      = errors.New("run lease ownership or generation changed")
	ErrRunWorkspaceBound = errors.New("run workspace is already bound to another host path")
)

// deps 是所有领域 Repo 共享的依赖（repo 层的共享依赖只有 gorm 句柄）。
type deps struct {
	DB *gorm.DB
}

func (d *deps) isMySQL() bool {
	return d.DB != nil && d.DB.Dialector.Name() == "mysql"
}

// sqlJSONText returns a portable "extract text at JSON path" expression.
// MySQL needs the JSON_UNQUOTE wrapper; sqlite's json_extract already returns
// unquoted text for string values.
func (d *deps) sqlJSONText(path string) string {
	if d.isMySQL() {
		return "JSON_UNQUOTE(JSON_EXTRACT(payload, ?))"
	}
	return "json_extract(payload, ?)"
}

// sqlPayloadJSONText is the literal-path variant of sqlJSONText for SELECT
// projections that cannot bind the path as a parameter. path is the JSON path
// below "$." and must be a code constant, never user input.
func (d *deps) sqlPayloadJSONText(path string) string {
	if d.isMySQL() {
		return "JSON_UNQUOTE(JSON_EXTRACT(payload, '$." + path + "'))"
	}
	return "json_extract(payload, '$." + path + "')"
}

// sqlExecutionContextJSONText is the execution_context-column counterpart of
// sqlPayloadJSONText. path is the JSON path below "$." and must be a code
// constant, never user input.
func (d *deps) sqlExecutionContextJSONText(path string) string {
	if d.isMySQL() {
		return "JSON_UNQUOTE(JSON_EXTRACT(execution_context, '$." + path + "'))"
	}
	return "json_extract(execution_context, '$." + path + "')"
}

// locking returns the query with a row-lock clause on MySQL. sqlite serializes
// writers already (single connection in tests), so no clause is needed there.
func (d *deps) locking(q *gorm.DB) *gorm.DB {
	if d.isMySQL() {
		return q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return q
}

// CatalogRepo owns the reusable definitions: agents, rules, skills, MCP
// servers, executors, sandboxes (with their Dockerfile or install), workflows,
// repositories, sync config, schedule jobs and definition revisions.
type CatalogRepo struct {
	*deps
	projects      *ProjectsRepo
	notifications *NotificationsRepo
	*BotInteractionsRepo
}

// ProjectsRepo owns projects and their members.
type BusinessDomainsRepo struct {
	*deps
}

type ProjectsRepo struct {
	*deps
}

// IssuesRepo owns issues and issue comments.
type IssuesRepo struct {
	*deps
	mu       sync.Mutex
	projects *ProjectsRepo
	users    *UsersRepo
	tasks    *TasksRepo
}

// TasksRepo owns tasks (the cross-instance idempotency records).
type TasksRepo struct {
	*deps
}

// UsersRepo owns users and their saved credentials.
type UsersRepo struct {
	*deps
	gitHTTPHosts *GitHTTPHostTrustRepo
	// keyring seals Bot credentials; installed by SetCredentialKeyring.
	keyringMu sync.RWMutex
	keyring   *credcrypt.Keyring
}

// TokensRepo owns API tokens.
type TokensRepo struct {
	*deps
}

// AssistantRepo owns Studio AI assistant conversations and scheduled prompts.
type AssistantRepo struct {
	*deps
	users         *UsersRepo
	notifications *NotificationsRepo
	*BotInteractionsRepo
}

// RunsRepo owns runs, their children (nodes/events/telemetry/logs/artifacts),
// run leases and workflow metrics.
type RunsRepo struct {
	*deps
	assistant     *AssistantRepo
	issues        *IssuesRepo
	catalog       *CatalogRepo
	runners       *RunnersRepo
	notifications *NotificationsRepo
	*BotInteractionsRepo
}

// RunnersRepo owns the CLI runner registry, runner tasks and runner events.
type RunnersRepo struct {
	*deps
	projects *ProjectsRepo
}

// DeliveryRepo owns MR delivery state and the delivery locking that follows.
type DeliveryRepo struct {
	*deps
	issues  *IssuesRepo
	tasks   *TasksRepo
	catalog *CatalogRepo
	runs    *RunsRepo
}

// AnalyticsRepo owns the read-only operational analytics view.
type AnalyticsRepo struct {
	*deps
	runs     *RunsRepo
	catalog  *CatalogRepo
	delivery *DeliveryRepo
	issues   *IssuesRepo
	tasks    *TasksRepo
	projects *ProjectsRepo
	insights *InsightsRepo
}

// Store 组合各领域 Repo。领域 Repo 以匿名指针嵌入（未导出类型）：包外仅能
// 通过提升的方法调用，与现有调用形态完全兼容（方法提升对未导出类型同样生效）。
type Store struct {
	*deps
	*CatalogRepo
	*ProjectsRepo
	*ProjectCredentialsRepo
	*BusinessDomainsRepo
	*IssuesRepo
	*TasksRepo
	*UsersRepo
	*TokensRepo
	*AssistantRepo
	*RunsRepo
	*RunnersRepo
	*DeliveryRepo
	*AnalyticsRepo
	*InsightsRepo
	*PreviewsRepo
	*EngineInstancesRepo
	*RoomsRepo
	*SpacesRepo
	*KnowledgeRepo
	*PlatformSettingsRepo
	*SSHHostTrustRepo
	*HTTPSHostTrustRepo
	*GitHTTPHostTrustRepo
	*NotificationsRepo
	*BotInteractionsRepo
	*EvolutionRepo
}

// newStore wires the shared deps and every cross-domain pointer. Domains have
// no cycles, so a single construction pass is enough.
func newStore(db *gorm.DB) *Store {
	d := &deps{DB: db}
	s := &Store{deps: d}
	s.ProjectsRepo = &ProjectsRepo{deps: d}
	s.ProjectCredentialsRepo = &ProjectCredentialsRepo{deps: d, projects: s.ProjectsRepo}
	s.BusinessDomainsRepo = &BusinessDomainsRepo{deps: d}
	s.GitHTTPHostTrustRepo = &GitHTTPHostTrustRepo{deps: d}
	s.UsersRepo = &UsersRepo{deps: d, gitHTTPHosts: s.GitHTTPHostTrustRepo}
	s.BotInteractionsRepo = &BotInteractionsRepo{deps: d}
	s.TokensRepo = &TokensRepo{deps: d}
	s.AssistantRepo = &AssistantRepo{deps: d, users: s.UsersRepo}
	s.TasksRepo = &TasksRepo{deps: d}
	s.NotificationsRepo = &NotificationsRepo{deps: d, users: s.UsersRepo, projects: s.ProjectsRepo, interactions: s.BotInteractionsRepo}
	s.AssistantRepo.notifications = s.NotificationsRepo
	s.CatalogRepo = &CatalogRepo{deps: d, projects: s.ProjectsRepo, notifications: s.NotificationsRepo}
	s.IssuesRepo = &IssuesRepo{deps: d, projects: s.ProjectsRepo, users: s.UsersRepo, tasks: s.TasksRepo}
	s.NotificationsRepo.issues = s.IssuesRepo
	s.RunnersRepo = &RunnersRepo{deps: d, projects: s.ProjectsRepo}
	s.RunsRepo = &RunsRepo{deps: d, assistant: s.AssistantRepo, issues: s.IssuesRepo, catalog: s.CatalogRepo, runners: s.RunnersRepo, notifications: s.NotificationsRepo}
	s.DeliveryRepo = &DeliveryRepo{deps: d, issues: s.IssuesRepo, tasks: s.TasksRepo, catalog: s.CatalogRepo, runs: s.RunsRepo}
	s.InsightsRepo = &InsightsRepo{deps: d}
	s.AnalyticsRepo = &AnalyticsRepo{deps: d, runs: s.RunsRepo, catalog: s.CatalogRepo, delivery: s.DeliveryRepo, issues: s.IssuesRepo, tasks: s.TasksRepo, projects: s.ProjectsRepo, insights: s.InsightsRepo}
	s.PreviewsRepo = &PreviewsRepo{deps: d}
	s.EngineInstancesRepo = &EngineInstancesRepo{deps: d}
	s.RoomsRepo = &RoomsRepo{deps: d}
	s.SpacesRepo = &SpacesRepo{deps: d}
	s.KnowledgeRepo = &KnowledgeRepo{deps: d}
	s.EvolutionRepo = &EvolutionRepo{deps: d, catalog: s.CatalogRepo}
	s.PlatformSettingsRepo = &PlatformSettingsRepo{deps: d}
	s.SSHHostTrustRepo = &SSHHostTrustRepo{deps: d}
	s.HTTPSHostTrustRepo = &HTTPSHostTrustRepo{deps: d}
	return s
}

// The datasources Open accepts. MySQL and SQLite are peers: the same gorm
// code paths serve both.
const (
	datasourceMySQL  = "mysql"
	datasourceSQLite = "sqlite"
)

// datasourceKind reports which datasource a DSN names.
//
// The two DSN shapes look nothing alike, so the rule is by shape:
//
//   - "file:" prefix        → SQLite. This is the explicit form: it carries
//     the shared in-memory database ("file::memory:?cache=shared") and any
//     URI parameters.
//   - contains "@"          → MySQL (the user[:password]@ separator).
//   - contains "(" or "://" → MySQL (the protocol(address)/dbname form).
//   - anything else         → SQLite file path.
//
// A SQLite path that would be read as a MySQL DSN (it contains "@", "(" or
// "://") must use the "file:" prefix; a MySQL DSN without credentials
// ("/dbname") must use the "tcp(host:port)/dbname" form.
func datasourceKind(dsn string) string {
	if strings.HasPrefix(dsn, "file:") {
		return datasourceSQLite
	}
	if strings.Contains(dsn, "@") || strings.Contains(dsn, "(") || strings.Contains(dsn, "://") {
		return datasourceMySQL
	}
	return datasourceSQLite
}

// Open connects the store to the configured datasource and completes the
// incremental DDL that database is missing. The DSN names either a MySQL
// database or a SQLite file; datasourceKind tells them apart.
//
// An empty DSN is a configuration error, not a reason to invent state
// somewhere else: the caller passes its flag value, and AGENTFLOW_DATASOURCE_DSN
// (any datasource) and AGENTFLOW_MYSQL_DSN (MySQL) are the environment
// fallbacks.
func Open(dsn string) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("AGENTFLOW_DATASOURCE_DSN"))
	}
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("AGENTFLOW_MYSQL_DSN"))
	}
	if dsn == "" {
		return nil, errors.New("数据源未配置：请设置 AGENTFLOW_DATASOURCE_DSN（SQLite 文件路径或 MySQL DSN）")
	}
	var (
		db  *gorm.DB
		err error
	)
	if datasourceKind(dsn) == datasourceSQLite {
		db, err = openSQLite(dsn)
	} else {
		db, err = openMySQL(dsn)
	}
	if err != nil {
		return nil, err
	}
	s := newStore(db)
	// Incremental DDL only (missing columns/tables/indexes) via gorm's
	// migrator. Schema baselines, data migrations and seed data are NOT run
	// from code: they are plain SQL scripts under migrations/, applied
	// manually by the operator.
	if err := autoMigrateIncremental(s.DB); err != nil {
		s.Close()
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	return s, nil
}

// openMySQL connects to a MySQL database. The DSN keeps the charset and
// parseTime settings the schema needs.
func openMySQL(dsn string) (*gorm.DB, error) {
	if !strings.Contains(dsn, "charset=") {
		if strings.Contains(dsn, "?") {
			dsn += "&charset=utf8mb4&collation=utf8mb4_unicode_ci"
		} else {
			dsn += "?charset=utf8mb4&collation=utf8mb4_unicode_ci"
		}
	}
	if !strings.Contains(dsn, "parseTime=") {
		dsn += "&parseTime=true&loc=UTC"
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(32)
		sqlDB.SetMaxIdleConns(8)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		if err := sqlDB.Ping(); err != nil {
			return nil, fmt.Errorf("mysql ping: %w", err)
		}
	}
	return db, nil
}

// sqliteBusyTimeout is the wait a SQLite connection spends on a locked
// database before it gives up.
const sqliteBusyTimeout = "10000"

// openSQLite connects to a SQLite database. The target is either a "file:"
// URI (which may be the shared in-memory database) or a filesystem path; a
// path's parent directory is created, so a fresh install needs one file and
// nothing else.
func openSQLite(dsn string) (*gorm.DB, error) {
	target, err := sqliteTarget(dsn)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(target), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sqlite handle: %w", err)
	}
	// SQLite serializes writers. One connection keeps a transaction and the
	// queries around it from interleaving, and matches the in-process test
	// store that the whole suite runs against.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}
	return db, nil
}

// sqliteTarget turns a SQLite DSN into the driver DSN. A "file:" URI is
// passed through; anything else is a filesystem path. Either way the file's
// parent directory is created, so a fresh install needs one file and nothing
// else — the "file:" form is what scripts/configure.sh writes, so it must
// behave exactly like the bare path.
func sqliteTarget(dsn string) (string, error) {
	target := dsn
	if !strings.HasPrefix(dsn, "file:") {
		abs, err := filepath.Abs(dsn)
		if err != nil {
			return "", fmt.Errorf("sqlite path %q: %w", dsn, err)
		}
		target = "file:" + abs
	}
	if dir, ok := sqliteParentDir(target); ok {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("sqlite directory %q: %w", dir, err)
		}
	}
	// Every connection waits for a busy database instead of failing at once.
	if strings.Contains(target, "?") {
		target += "&_pragma=busy_timeout(" + sqliteBusyTimeout + ")"
	} else {
		target += "?_pragma=busy_timeout(" + sqliteBusyTimeout + ")"
	}
	return target, nil
}

// sqliteParentDir returns the directory that must exist for the SQLite file
// behind a "file:" URI. It reports false for the in-memory forms, which have
// no directory: "file::memory:..." and the empty-name form.
func sqliteParentDir(target string) (string, bool) {
	path := strings.TrimPrefix(target, "file:")
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if path == "" || path == ":memory:" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	dir := filepath.Dir(abs)
	if dir == "" {
		return "", false
	}
	return dir, true
}

// Close releases the underlying connection pool. The gorm handle is kept so
// in-flight background goroutines fail gracefully ("sql: database is closed")
// instead of panicking on a nil handle.
func (s *Store) Close() {
	if s.DB == nil {
		return
	}
	if sqlDB, err := s.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func (s *Store) StorageInfo() map[string]any {
	driver := ""
	if s.DB != nil {
		driver = s.DB.Dialector.Name()
	}
	note := "所有状态都直接读写数据源：项目、需求、运行记录、助手对话和定时任务都在同一个库里。"
	switch driver {
	case datasourceSQLite:
		note = "所有状态都在 SQLite 文件里：项目、需求、运行记录、助手对话和定时任务都直接读写这个文件。"
	case datasourceMySQL:
		note = "所有状态都在 MySQL 里：项目、需求、运行记录、助手对话和定时任务都直接读写数据库。"
	}
	return map[string]any{
		"driver":   driver,
		"database": true,
		"note":     note,
	}
}

func IssueBoundToTracker(issue domain.Issue) bool {
	if issue.Source != "gongfeng" && issue.Source != "tapd" {
		return false
	}
	if issue.ExternalRef == nil {
		return false
	}
	return strings.TrimSpace(issue.ExternalRef.ExternalID) != "" || issue.ExternalRef.ExternalIID != 0
}

// RestoreTrackerBinding fills Source/ExternalRef from a still-bound snapshot, or
// from a stable local id (tapd-… / gf-…) after a save wiped the pointer.
func RestoreTrackerBinding(issue *domain.Issue, existing domain.Issue) {
	if issue == nil || IssueBoundToTracker(*issue) {
		return
	}
	if IssueBoundToTracker(existing) {
		issue.Source = existing.Source
		issue.ExternalRef = existing.ExternalRef
		if strings.TrimSpace(issue.ID) == "" {
			issue.ID = existing.ID
		}
		return
	}
	id := strings.TrimSpace(issue.ID)
	if id == "" {
		id = strings.TrimSpace(existing.ID)
		issue.ID = id
	}
	if extID, ok := strings.CutPrefix(id, "tapd-"); ok && strings.TrimSpace(extID) != "" {
		issue.Source = "tapd"
		ref := domain.ExternalRef{Source: "tapd", ExternalID: extID, ExternalType: "story"}
		if existing.ExternalRef != nil {
			ref.ExternalURL = existing.ExternalRef.ExternalURL
			ref.ExternalState = existing.ExternalRef.ExternalState
		}
		issue.ExternalRef = &ref
		return
	}
	if _, iid := parseStoredGongfengIssueID(id); iid > 0 {
		issue.Source = "gongfeng"
		if existing.ExternalRef != nil {
			issue.ExternalRef = existing.ExternalRef
			return
		}
		issue.ExternalRef = &domain.ExternalRef{Source: "gongfeng", ExternalIID: iid, ExternalType: "issue"}
	}
}

func parseStoredGongfengIssueID(id string) (repo string, iid int) {
	body := strings.TrimPrefix(strings.TrimSpace(id), "gf-")
	if body == "" || body == strings.TrimSpace(id) {
		return "", 0
	}
	i := strings.LastIndex(body, "-")
	if i <= 0 {
		return "", 0
	}
	n := 0
	for _, ch := range body[i+1:] {
		if ch < '0' || ch > '9' {
			return "", 0
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return "", 0
	}
	return body[:i], n
}

func cloneRunNodes(nodes map[string]*domain.NodeRun) map[string]*domain.NodeRun {
	if len(nodes) == 0 {
		return nil
	}
	out := make(map[string]*domain.NodeRun, len(nodes))
	for id, node := range nodes {
		if node == nil {
			continue
		}
		cp := *node
		out[id] = &cp
	}
	return out
}

func cloneRun(run domain.Run) domain.Run {
	run.Nodes = cloneRunNodes(run.Nodes)
	run.Events = append([]domain.RunEvent(nil), run.Events...)
	run.Telemetry = append([]domain.TelemetryEvent(nil), run.Telemetry...)
	run.Logs = append([]domain.LogRecord(nil), run.Logs...)
	run.Artifacts = append([]domain.Artifact(nil), run.Artifacts...)
	return run
}

func normalizeRefs(values []string, prefix string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimPrefix(v, prefix)
		v = strings.TrimSuffix(v, ".md")
		out = append(out, v)
	}
	return out
}

func sortSlice[T any](values []T, less func(a, b T) bool) {
	sort.Slice(values, func(i, j int) bool { return less(values[i], values[j]) })
}

func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"agentflow/internal/domain"
	"agentflow/internal/repo"

	"github.com/gin-gonic/gin"
)

// staticPublicURL is a fixed external address for handlers that resolve it on
// each use.
func staticPublicURL(address string) func() string {
	return func() string { return address }
}

// stubCLIModelDiscovery keeps tests from running the CodeBuddy CLI.
func stubCLIModelDiscovery(t *testing.T) {
	t.Helper()
	old := discoverCLIModels
	discoverCLIModels = func(context.Context, string, string) []string { return nil }
	cliModelMu.Lock()
	cliModelCache = map[string]cliModelEntry{}
	cliModelMu.Unlock()
	t.Cleanup(func() {
		discoverCLIModels = old
		cliModelMu.Lock()
		cliModelCache = map[string]cliModelEntry{}
		cliModelMu.Unlock()
	})
}

// cvAStore opens an in-process store for a handler test.
func cvAStore(t *testing.T) *repo.Store {
	t.Helper()
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	return st
}

// cvAContext builds a gin context for a handler test.
func cvAContext(t *testing.T, method, target, body string, user *domain.User) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != nil {
		req = req.WithContext(context.WithValue(req.Context(), requestUserContextKey{}, domainUserToAuth(*user)))
	}
	c.Request = req
	return c, rec
}

// sharedWorkbenchFixture builds a store with an owner, a peer and an
// outsider, plus one run that holds a shared workspace.
func sharedWorkbenchFixture(t *testing.T) (*deps, domain.User, domain.User, domain.User, string) {
	t.Helper()
	h, owner := workbenchFixture(t)
	for _, name := range []string{"peer", "outsider"} {
		if err := h.repo.PutUser(domain.User{LoginName: name, CodeBuddyAPIKey: name + "-test-key"}); err != nil {
			t.Fatal(err)
		}
	}
	peer, _ := h.repo.UserByLoginName("peer")
	out, _ := h.repo.UserByLoginName("outsider")
	p, _ := h.repo.Project("workbench")
	// One account owns the platform; there is no roster to extend.
	if err := h.repo.PutProject(p); err != nil {
		t.Fatal(err)
	}
	r := domain.Run{ID: "shared-run", WorkflowID: "wf", WorkflowSnapshot: domain.WorkflowDefinition{ID: "wf", Name: "开发流程", Nodes: []domain.WorkflowNode{{ID: "develop", Type: "agent", AgentID: "dev"}}}, ExecutionContext: domain.TelemetryContext{ProjectID: p.ID}, Status: "waiting", Nodes: map[string]*domain.NodeRun{}}
	if err := h.repo.PutRun(r); err != nil {
		t.Fatal(err)
	}
	return h, owner, peer, out, r.ID
}

// loopbackMySQLDSN reports whether a DSN points at a local MySQL.
func loopbackMySQLDSN(dsn string) bool {
	return strings.Contains(dsn, "tcp(127.0.0.1:") || strings.Contains(dsn, "tcp(localhost:")
}

package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentflow/internal/domain"
	"github.com/gin-gonic/gin"
)

func TestKnowledgeHTTPBranches(t *testing.T) {
	_, h, _, mcpURL := newKnowledgeTestServer(t)
	created := doJSON(t, h, http.MethodPost, "/api/knowledge/connectors", `{"name":"Local MCP","type":"mcp","endpoint":"`+mcpURL+`"}`, nil)
	if created.Code != 201 {
		t.Fatalf("POST /api/knowledge/connectors = %d: %s", created.Code, created.Body.String())
	}
	var conn domain.KnowledgeConnector
	_ = json.Unmarshal(created.Body.Bytes(), &conn)

	for _, tc := range []struct {
		method, path, body string
		code               int
	}{
		{http.MethodGet, "/api/knowledge/connector-types", "", 200},
		{http.MethodGet, "/api/knowledge/connectors", "", 200},
		{http.MethodPost, "/api/knowledge/connectors", `{`, 400},
		{http.MethodGet, "/api/knowledge/connectors/" + conn.ID, "", 200},
		{http.MethodGet, "/api/knowledge/connectors/missing", "", 404},
		{http.MethodPut, "/api/knowledge/connectors/missing", `{}`, 404},
		{http.MethodDelete, "/api/knowledge/connectors/missing", "", 404},
		{http.MethodPost, "/api/knowledge/connectors/" + conn.ID + "/check", `{}`, 200},
		{http.MethodGet, "/api/knowledge/connectors/" + conn.ID + "/sources", "", 200},
		{http.MethodGet, "/api/knowledge/reference-options", "", 200},
		{http.MethodPost, "/api/knowledge/reference-check", `{`, 400},
	} {
		rec := doJSON(t, h, tc.method, tc.path, tc.body, nil)
		if rec.Code != tc.code {
			t.Errorf("%s %s = %d, want %d: %s", tc.method, tc.path, rec.Code, tc.code, rec.Body.String())
		}
	}
}

func TestGateMCPBinaryConfiguredArtifactAndErrors(t *testing.T) {
	run := func(query string) (int, string) {
		c, rec := cvAContext(t, http.MethodGet, "/api/gate?"+query, "", nil)
		serveGateMCPBinary(c)
		return rec.Code, rec.Body.String()
	}
	if code, _ := run("os=windows&arch=amd64"); code != http.StatusBadRequest {
		t.Fatalf("unsupported OS=%d", code)
	}
	if code, _ := run("os=linux&arch=mips"); code != http.StatusBadRequest {
		t.Fatalf("unsupported arch=%d", code)
	}
	t.Setenv("AGENTFLOW_GATE_MCP_BINARY", filepath.Join(t.TempDir(), "{os}-{arch}"))
	if code, _ := run("os=linux&arch=amd64"); code != http.StatusNotFound {
		t.Fatalf("missing artifact=%d", code)
	}
	path := filepath.Join(t.TempDir(), "gate-linux-amd64")
	if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTFLOW_GATE_MCP_BINARY", strings.ReplaceAll(path, "linux-amd64", "{os}-{arch}"))
	code, body := run("os=linux&arch=amd64")
	if code != http.StatusOK || body != "binary" {
		t.Fatalf("artifact=%d %q", code, body)
	}
}

func TestMCPMarketFileAndBotIdentityErrors(t *testing.T) {
	st := cvAStore(t)
	_ = st.PutProject(domain.Project{ID: "mcp-p", Name: "MCP"})
	market := &marketHandlers{deps: &deps{repo: st}}
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"scope":"bad","server":{"id":"x"}}`, 400},
		{`{"scope":"project","server":{"id":"x"}}`, 400},
		{`{"scope":"project","projectId":"missing","server":{"id":"x"}}`, 404},
		{`{"scope":"project","projectId":"mcp-p","server":{}}`, 400},
		{`{"scope":"personal","server":{"id":"x"}}`, 401},
	} {
		c, rec := cvAContext(t, http.MethodPost, "/api/mcp-servers/custom", tc.body, nil)
		market.mcpCustomInstall(c)
		if rec.Code != tc.code {
			t.Errorf("custom %s = %d: %s", tc.body, rec.Code, rec.Body.String())
		}
	}
	if err := st.PutMCPServer(domain.MCPServerDefinition{ID: "market", Name: "market", MarketID: "70", Scope: "project", ProjectID: "mcp-p", Transport: "http", URL: "https://example.test/mcp"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutMCPServer(domain.MCPServerDefinition{ID: "custom", Name: "custom", Scope: "project", ProjectID: "mcp-p", Transport: "http", URL: "https://example.test/custom"}); err != nil {
		t.Fatal(err)
	}
	mcp := &mcpHandlers{deps: &deps{repo: st}}
	for _, tc := range []struct {
		id, path, method string
		code             int
	}{
		{"missing", "x", http.MethodGet, 404},
		{"market", "x", http.MethodGet, 400},
		{"custom", "../x", http.MethodGet, 400},
		{"custom", "missing.txt", http.MethodGet, 404},
		{"custom", "x", http.MethodPatch, 405},
	} {
		c, rec := cvAContext(t, tc.method, "/api/mcp/"+tc.id+"/"+tc.path, "", nil)
		c.Params = ginParams(tc.id, tc.path)
		mcp.mcpFile(c)
		if rec.Code != tc.code {
			t.Errorf("mcp file %s = %d: %s", tc.path, rec.Code, rec.Body.String())
		}
	}

	_ = st.PutUser(domain.User{LoginName: "bot-user"})
	user, _ := st.UserByLoginName("bot-user")
	ident := &botIdentityHandlers{deps: &deps{repo: st}, publicURL: staticPublicURL("https://example.com")}
	if botIdentityCommand("@bot /bind abc") != "/bind abc" {
		t.Fatal("bot command")
	}
	for _, tc := range []struct {
		user         *domain.User
		body, origin string
		code         int
	}{
		{nil, `{}`, "", 401},
		{&user, `{"token":"short"}`, "", 400},
	} {
		c, rec := cvAContext(t, http.MethodPost, "/api/bot/claim", tc.body, tc.user)
		c.Request.Host = "host"
		c.Request.Header.Set("Origin", tc.origin)
		ident.binding(c)
		if rec.Code != tc.code {
			t.Errorf("binding=%d %s", rec.Code, rec.Body.String())
		}
	}
	c, rec := cvAContext(t, http.MethodGet, "/api/bot/identities", "", &user)
	ident.list(c)
	if rec.Code != 200 {
		t.Fatalf("identity list=%d", rec.Code)
	}
	c, rec = cvAContext(t, http.MethodDelete, "/api/bot/identities/missing", "", &user)
	c.Params = ginParams("missing", "")
	ident.remove(c)
	if rec.Code != 403 {
		t.Fatalf("cross origin remove=%d", rec.Code)
	}
	if ok, msg, err := ident.message(domain.NotificationBot{BotID: "b"}, domain.BotInteraction{SenderID: "s", Content: "hello"}); !ok || err != nil || !strings.Contains(msg, "关联") {
		t.Fatalf("unbound message=%v %q %v", ok, msg, err)
	}
	ident.publicURL = staticPublicURL("bad")
	if ok, msg, _ := ident.message(domain.NotificationBot{BotID: "b"}, domain.BotInteraction{SenderID: "s2", Content: "hello"}); !ok || !strings.Contains(msg, "外部访问地址") {
		t.Fatalf("bad public URL=%v %q", ok, msg)
	}
}

func ginParams(id, path string) []gin.Param {
	out := []gin.Param{{Key: "id", Value: id}}
	if path != "" {
		out = append(out, gin.Param{Key: "path", Value: path})
	}
	return out
}

func TestInsightsValidationAndHTTP(t *testing.T) {
	now := time.Now().UTC()
	cost := 1.0
	base := domain.InsightExecution{
		ID: "id", Type: "workflow_execution", ExternalExecutionID: "external",
		Workflow:  domain.InsightWorkflow{ID: "wf", Name: "Workflow"},
		StartedAt: now.Add(-time.Minute), FinishedAt: now, Status: "succeeded",
		Usage: []domain.InsightUsage{{Provider: "p", Model: "m", InputTokens: 10, OutputTokens: 5, CostStatus: "reported", CostUSD: &cost}},
	}
	if reason := validateInsightExecution(&base, now); reason != "" {
		t.Fatalf("valid insight: %s", reason)
	}
	cases := []func(*domain.InsightExecution){
		func(v *domain.InsightExecution) { v.Type = "bad" },
		func(v *domain.InsightExecution) { v.ID = "" },
		func(v *domain.InsightExecution) { v.Status = "running" },
		func(v *domain.InsightExecution) { v.FinishedAt = v.StartedAt.Add(-time.Second) },
		func(v *domain.InsightExecution) { x := int64(-1); v.ExecutionDurationMs = &x },
		func(v *domain.InsightExecution) { v.URL = "ftp://bad" },
		func(v *domain.InsightExecution) { v.Usage = nil },
		func(v *domain.InsightExecution) { v.Usage[0].Provider = "" },
		func(v *domain.InsightExecution) { v.Usage = append(v.Usage, v.Usage[0]) },
		func(v *domain.InsightExecution) { v.Usage[0].InputTokens = -1 },
		func(v *domain.InsightExecution) { v.Usage[0].CacheReadTokens = 11 },
		func(v *domain.InsightExecution) { v.Usage[0].CostStatus = "unknown" },
		func(v *domain.InsightExecution) { v.Usage[0].CostStatus = "bad" },
	}
	for i, mutate := range cases {
		copy := base
		copy.Usage = append([]domain.InsightUsage{}, base.Usage...)
		mutate(&copy)
		if reason := validateInsightExecution(&copy, now); reason == "" {
			t.Errorf("invalid insight case %d accepted", i)
		}
	}

	st := cvAStore(t)
	_ = st.PutUser(domain.User{LoginName: "owner"})
	owner, _ := st.UserByLoginName("owner")
	_ = st.PutProject(domain.Project{ID: "insight-p", Name: "P", OwnerID: owner.ID})
	h := &insightsHandlers{deps: &deps{repo: st}}
	c, rec := cvAContext(t, http.MethodGet, "/api/insights/sources?projectId=insight-p", "", nil)
	h.sources(c)
	if rec.Code != 401 {
		t.Fatalf("anonymous sources=%d", rec.Code)
	}
	c, rec = cvAContext(t, http.MethodPost, "/api/insights/sources", `{"projectId":"insight-p","name":""}`, &owner)
	h.sources(c)
	if rec.Code != 400 {
		t.Fatalf("blank source=%d %s", rec.Code, rec.Body.String())
	}
	c, rec = cvAContext(t, http.MethodPost, "/api/insights/v1/ingest", `{}`, nil)
	h.ingest(c)
	if rec.Code != 401 {
		t.Fatalf("anonymous ingest=%d", rec.Code)
	}
	c, rec = cvAContext(t, http.MethodDelete, "/api/insights/sources/missing", "", &owner)
	c.Params = ginParams("missing", "")
	h.revokeSource(c)
	if rec.Code != 404 {
		t.Fatalf("missing revoke=%d", rec.Code)
	}
	raw, _ := json.Marshal(base)
	if !strings.Contains(string(raw), "workflow_execution") {
		t.Fatal("insight marshal")
	}
}


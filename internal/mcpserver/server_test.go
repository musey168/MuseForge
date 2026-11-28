package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentflow/internal/application"
)

func TestPublicOperationsAreUniqueAndExcludeInternalControlPlane(t *testing.T) {
	seen := map[string]bool{}
	for _, op := range application.PublicOperations() {
		if op.Name == "" || op.Method == "" || !strings.HasPrefix(op.Path, "/api/") {
			t.Fatalf("invalid operation: %+v", op)
		}
		if seen[op.Name] {
			t.Fatalf("duplicate operation name %q", op.Name)
		}
		seen[op.Name] = true
		for _, forbidden := range []string{"/signals", "/otel/", "/leases", "/sandboxes/reclaim"} {
			if strings.Contains(op.Path, forbidden) {
				t.Fatalf("internal operation %q is public", op.Path)
			}
		}
	}
	// The personal build serves the requirement list, the projects, the
	// credentials, the machine inventory, the usage console, the tracker
	// import and the model catalogue.
	if len(seen) < 30 {
		t.Fatalf("public operation count = %d, want at least 30", len(seen))
	}
}

func TestStreamableHTTPInitializeListAndCall(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects" || r.Method != http.MethodGet {
			t.Fatalf("unexpected API request %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[{"id":"platform","name":"Platform"}]`)
	})
	handler := New(LocalInvoker{Handler: api}).HTTPHandler()

	init := rpcPost(t, handler, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test","version":"1"}}}`)
	if init.Error != nil {
		t.Fatalf("initialize failed: %+v", init.Error)
	}
	raw, _ := json.Marshal(init.Result)
	if !bytes.Contains(raw, []byte(`"protocolVersion":"2025-06-18"`)) {
		t.Fatalf("unexpected initialize response: %s", raw)
	}

	list := rpcPost(t, handler, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	raw, _ = json.Marshal(list.Result)
	if !bytes.Contains(raw, []byte(`"projects_list"`)) || bytes.Contains(raw, []byte(`"runs_signals"`)) {
		t.Fatalf("unexpected tool list: %s", raw)
	}

	call := rpcPost(t, handler, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"projects_list","arguments":{}}}`)
	raw, _ = json.Marshal(call.Result)
	if !bytes.Contains(raw, []byte(`"Platform"`)) || !bytes.Contains(raw, []byte(`"isError":false`)) {
		t.Fatalf("unexpected tool result: %s", raw)
	}
}

func TestToolCallRendersPathQueryAndBody(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/issues/issue-1/comments" || r.URL.Query().Get("notify") != "true" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["body"] != "a note" {
			t.Fatalf("unexpected body: %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"ok":true}`)
	})
	server := New(LocalInvoker{Handler: api})
	params := json.RawMessage(`{"name":"issues_comments_add","arguments":{"path":{"id":"issue-1"},"query":{"notify":"true"},"body":{"body":"a note"}}}`)
	result, err := server.callTool(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result["isError"] != false {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestToolCallReturnsStructuredAPIError(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"error":"run is not waiting"}`)
	})
	result, err := New(LocalInvoker{Handler: api}).callTool(context.Background(), json.RawMessage(`{"name":"issues_close","arguments":{"path":{"id":"issue-1"},"body":{"source":"manual"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if result["isError"] != true {
		t.Fatalf("expected tool error: %+v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	if structured["status"] != 409 {
		t.Fatalf("status = %#v", structured["status"])
	}
}

// The personal build publishes no resources: every one it had pointed at a
// run, and nothing runs. Reading one is refused rather than answered with the
// 404 body of a route that no longer exists.
func TestResourceReadsAreRefused(t *testing.T) {
	server := New(LocalInvoker{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("a resource read must not reach the API: %s", r.URL.Path)
	})})
	for _, uri := range []string{
		"agentflow://runs/run-1/artifacts/art-1",
		"agentflow://runs/run-1/logs",
	} {
		raw, _ := json.Marshal(map[string]string{"uri": uri})
		if _, err := server.readResource(context.Background(), raw); err == nil {
			t.Fatalf("%s was answered", uri)
		}
	}
}

func TestStdioAndRemoteBasicAuth(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "api-user" || pass != "api-pass" {
			t.Fatal("missing remote API Basic Auth")
		}
		io.WriteString(w, `{"status":"ok"}`)
	}))
	defer backend.Close()

	server := New(RemoteInvoker{BaseURL: backend.URL, Username: "api-user", Password: "api-pass"})
	in := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"health_get\",\"arguments\":{}}}\n")
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status":"ok"`) {
		t.Fatalf("unexpected stdio output: %s", out.String())
	}
}

func TestRemoteInvokerPrefersBearerToken(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer afp_test_token" {
			t.Fatalf("Authorization = %q", got)
		}
		io.WriteString(w, `{"status":"ok"}`)
	}))
	defer backend.Close()

	invoker := RemoteInvoker{
		BaseURL: backend.URL, Token: "afp_test_token",
		Username: "legacy-user", Password: "legacy-pass",
	}
	result, err := invoker.Invoke(context.Background(), application.PublicOperations()[0], ToolInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("status=%d body=%s", result.Status, result.Body)
	}
}

func rpcPost(t *testing.T, handler http.Handler, body string) rpcResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var response rpcResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestLocalInvokerForwardsIncomingAuth(t *testing.T) {
	var gotCookie, gotAuth string
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	})
	handler := New(LocalInvoker{Handler: api}).HTTPHandler()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"projects_list","arguments":{}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "agentflow_user=signed-session")
	req.Header.Set("Authorization", "Bearer afp_forward_me")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotCookie != "agentflow_user=signed-session" {
		t.Fatalf("Cookie = %q, want forwarded session cookie", gotCookie)
	}
	if gotAuth != "Bearer afp_forward_me" {
		t.Fatalf("Authorization = %q, want forwarded bearer", gotAuth)
	}
}

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentflow/internal/domain"

	"github.com/gorilla/websocket"
)

// The client talks to the four endpoints the registration client owns.
func TestClientRegistrationEndpoints(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/runners/register":
			if r.Header.Get("X-Bootstrap-Token") != "boot" {
				t.Errorf("register did not carry the bootstrap token: %v", r.Header)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runnerId": "runner-1", "sessionToken": "session-1",
				"expiresAt": time.Now().Add(time.Hour).UTC(),
			})
		case "/runners/heartbeat":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"expiresAt": time.Now().Add(time.Hour).UTC()})
		case "/runners/disconnect", "/runners/deregister":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := NewClient(server.URL)
	c.SetSession("session")
	ctx := context.Background()
	registered, err := c.Register(ctx, "boot", RegisterRequest{Name: "machine", Version: runnerVersion})
	if err != nil || registered.RunnerID != "runner-1" || registered.SessionToken != "session-1" {
		t.Fatalf("register = %+v, err = %v", registered, err)
	}
	heartbeat, err := c.Heartbeat(ctx, HeartbeatRequest{Status: "idle", Version: runnerVersion})
	if err != nil || heartbeat.ExpiresAt.IsZero() {
		t.Fatalf("heartbeat = %+v, err = %v", heartbeat, err)
	}
	if err := c.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Deregister(ctx); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/runners/register", "/runners/heartbeat", "/runners/disconnect", "/runners/deregister"} {
		var seen bool
		for _, got := range paths {
			if got == path {
				seen = true
			}
		}
		if !seen {
			t.Fatalf("%s was never called: %v", path, paths)
		}
	}
}

// The client maps the control plane's answers onto its own errors.
func TestClientMapsControlPlaneErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		case "/bad":
			http.Error(w, "bad request", http.StatusBadRequest)
		case "/invalid-json":
			_, _ = w.Write([]byte("{"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := NewClient(server.URL)
	ctx := context.Background()
	if err := c.do(ctx, http.MethodGet, "/unauthorized", nil, nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unauthorized = %v", err)
	}
	if err := c.do(ctx, http.MethodGet, "/bad", nil, nil); err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("bad = %v", err)
	}
	var out map[string]any
	if err := c.do(ctx, http.MethodGet, "/invalid-json", nil, &out); err == nil {
		t.Fatal("an invalid response was accepted")
	}
}

// The control socket advertises the SSH host-key probe and carries the
// session.
func TestClientOpenControlWebSocket(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runners/connect" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get(domain.RunnerSSHHostKeyProbeHeader) != "1" {
			http.Error(w, "missing SSH host key probe support", http.StatusBadRequest)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()
	c := NewClient(server.URL)
	if _, err := c.OpenControl(context.Background()); err == nil {
		t.Fatal("an unauthorized socket was opened")
	}
	c.SetSession("token")
	conn, err := c.OpenControl(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if _, err := NewClient("file:///tmp").OpenControl(context.Background()); err == nil {
		t.Fatal("an unsupported scheme was accepted")
	}
}

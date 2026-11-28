package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

func TestSecurityHeadersAndSameOriginCORS(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")

	req := httptest.NewRequest(http.MethodGet, "http://agentflow.example/api/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("X-Frame-Options") != "DENY" || rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("missing security headers: status=%d headers=%v", rec.Code, rec.Header())
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "worker-src 'self' blob:") {
		t.Fatalf("workspace editor workers are not allowed by CSP: %q", rec.Header().Get("Content-Security-Policy"))
	}
	next := httptest.NewRecorder()
	h.ServeHTTP(next, httptest.NewRequest(http.MethodGet, "http://agentflow.example/next/", nil))
	if next.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(next.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("/next/ is not a framed workbench: %v", next.Header())
	}

	foreign := httptest.NewRequest(http.MethodOptions, "http://agentflow.example/api/issues", nil)
	foreign.Header.Set("Origin", "https://evil.example")
	foreignRec := httptest.NewRecorder()
	h.ServeHTTP(foreignRec, foreign)
	if foreignRec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin should be denied, got %d", foreignRec.Code)
	}

	sameOrigin := httptest.NewRequest(http.MethodOptions, "http://agentflow.example/api/issues", nil)
	sameOrigin.Header.Set("Origin", "http://agentflow.example")
	sameOriginRec := httptest.NewRecorder()
	h.ServeHTTP(sameOriginRec, sameOrigin)
	if sameOriginRec.Code != http.StatusNoContent || sameOriginRec.Header().Get("Access-Control-Allow-Origin") != "http://agentflow.example" {
		t.Fatalf("same origin preflight failed: status=%d headers=%v", sameOriginRec.Code, sameOriginRec.Header())
	}
}

func TestGzipJSONResponses(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("missing gzip: %v", rec.Header())
	}
}

func requestJSON[T any](t *testing.T, h http.Handler, method, path string, body any, wantStatus int) T {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s: status %d, body %s", method, path, rec.Code, rec.Body.String())
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func requestEmptyJSON[T any](t *testing.T, h http.Handler, method, path string, wantStatus int) T {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s: status %d, body %s", method, path, rec.Code, rec.Body.String())
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

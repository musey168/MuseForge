package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The runner replaces its own binary only after the download matched the
// checksum the platform published.
func TestRunnerUpdateReplacesBinaryOnlyAfterVerification(t *testing.T) {
	newBinary := []byte("new runner")
	hash := sha256.Sum256(newBinary)
	expected := hex.EncodeToString(hash[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runners/agentflow-runner" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("metadata") == "1" {
			fmt.Fprintf(w, `{"sha256":%q}`, expected)
			return
		}
		_, _ = w.Write(newBinary)
	}))
	defer server.Close()
	d, err := New(Config{ServerURL: server.URL, BootstrapToken: "test"})
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agentflow-runner")
	if err := os.WriteFile(binary, []byte("old runner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := d.checkForUpdate(context.Background(), binary); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newBinary) || !d.updateReady.Load() {
		t.Fatalf("binary = %q, updateReady = %v", got, d.updateReady.Load())
	}
	// The installed build hands over to the daemon, which restarts the entry
	// point into the new binary.
	select {
	case <-d.restart:
	default:
		t.Fatal("the daemon was not asked to restart after the update")
	}
}

// A download that does not match the published checksum leaves the installed
// binary untouched.
func TestRunnerUpdateRejectsChecksumMismatch(t *testing.T) {
	newBinary := []byte("new runner")
	hash := sha256.Sum256(newBinary)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("metadata") == "1" {
			fmt.Fprintf(w, `{"sha256":%q}`, hex.EncodeToString(hash[:]))
			return
		}
		_, _ = w.Write([]byte("different payload"))
	}))
	defer server.Close()
	d, err := New(Config{ServerURL: server.URL, BootstrapToken: "test"})
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agentflow-runner")
	if err := os.WriteFile(binary, []byte("old runner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := d.checkForUpdate(context.Background(), binary); err == nil {
		t.Fatal("the checksum mismatch was accepted")
	}
	got, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old runner" || d.updateReady.Load() {
		t.Fatalf("the binary changed after the failed verification: %q", got)
	}
}

// The automatic update is off when the machine asked for it.
func TestRunnerUpdateRespectsDisabledAutoUpdate(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	d, err := New(Config{ServerURL: server.URL, BootstrapToken: "test", DisableAutoUpdate: true})
	if err != nil {
		t.Fatal(err)
	}
	d.checkRunnerUpdate(context.Background())
	if requests != 0 {
		t.Fatalf("the disabled auto-update asked the platform %d times", requests)
	}
}

// A session renewal is written to disk, so a restart keeps the identity.
func TestHeartbeatRenewalPersistsExistingRunnerIdentity(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	d, err := New(Config{ServerURL: "https://runner.example", BootstrapToken: "one-time-code", StatePath: statePath})
	if err != nil {
		t.Fatal(err)
	}
	oldExpiry := time.Now().Add(6 * 24 * time.Hour)
	if err := d.saveState(daemonState{RunnerID: "runner-1", SessionToken: "session-1", ExpiresAt: oldExpiry}); err != nil {
		t.Fatal(err)
	}
	newExpiry := time.Now().Add(30 * 24 * time.Hour)
	d.refreshSessionExpiry(newExpiry)
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved daemonState
	if err := json.Unmarshal(got, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.RunnerID != "runner-1" || saved.SessionToken != "session-1" || !saved.ExpiresAt.Equal(newExpiry) {
		t.Fatalf("renewed state = %+v", saved)
	}
}

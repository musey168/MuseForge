package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentflow/internal/domain"

	"github.com/gorilla/websocket"
)

// controlServer answers one control request and hands the runner's answer
// back to the test.
func controlServer(t *testing.T, request controlMessage) controlResponse {
	t.Helper()
	upgrader := websocket.Upgrader{}
	answers := make(chan controlResponse, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if err := conn.WriteJSON(request); err != nil {
			return
		}
		var answer controlMessage
		if err := conn.ReadJSON(&answer); err != nil {
			return
		}
		if answer.WorkspaceResponse != nil {
			answers <- *answer.WorkspaceResponse
		}
	}))
	t.Cleanup(server.Close)
	d, err := New(Config{ServerURL: server.URL, BootstrapToken: "boot", StatePath: t.TempDir() + "/state.json", DisableAutoUpdate: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go d.controlLoop(ctx)
	select {
	case answer := <-answers:
		return answer
	case <-time.After(10 * time.Second):
		t.Fatal("the runner did not answer the control request")
		return controlResponse{}
	}
}

// The maintenance operation answers with the machine's own update state.
func TestControlAnswersMaintenanceOperation(t *testing.T) {
	answer := controlServer(t, controlMessage{
		Type:      "workspace.request",
		RequestID: "req-1",
		WorkspaceRequest: &controlRequest{
			Operation:   controlOperationMaintenance,
			Maintenance: &domain.RunnerMaintenanceRequest{StatusOnly: true},
		},
	})
	if answer.Status != http.StatusOK {
		t.Fatalf("answer = %+v", answer)
	}
	var result domain.RunnerMaintenanceResult
	if err := json.Unmarshal(answer.Payload, &result); err != nil {
		t.Fatalf("payload = %s, err = %v", answer.Payload, err)
	}
	if result.Status == "" {
		t.Fatalf("maintenance result = %+v", result)
	}
}

// The model probe answers with the models the machine's CLI can reach.
func TestControlAnswersLoginModelsOperation(t *testing.T) {
	answer := controlServer(t, controlMessage{
		Type:      "workspace.request",
		RequestID: "req-2",
		WorkspaceRequest: &controlRequest{
			Operation:     controlOperationLoginModels,
			ModelProvider: "codex",
		},
	})
	// The fixture machine has no CLI installed, so the probe reports that
	// the CLI is unavailable instead of inventing a catalogue.
	if answer.Status != http.StatusNotFound {
		t.Fatalf("answer = %+v", answer)
	}
	if !strings.Contains(answer.Error, "CLI") {
		t.Fatalf("answer = %+v", answer)
	}
}

// Anything the machine inventory no longer serves is refused, not ignored.
func TestControlRefusesUnknownOperation(t *testing.T) {
	answer := controlServer(t, controlMessage{
		Type:      "workspace.request",
		RequestID: "req-3",
		WorkspaceRequest: &controlRequest{
			Operation: "terminal",
		},
	})
	if answer.Status != http.StatusBadRequest || answer.Error != "unsupported runner operation" {
		t.Fatalf("answer = %+v", answer)
	}
}

// The SSH host-key probe reads the host key from the machine and answers with
// it, so the platform can trust the host.
func TestControlAnswersSSHHostKeyOperation(t *testing.T) {
	answer := controlServer(t, controlMessage{
		Type:      "workspace.request",
		RequestID: "req-4",
		WorkspaceRequest: &controlRequest{
			Operation:         controlOperationSSHHostKey,
			SSHHostKeyAddress: "127.0.0.1:1",
		},
	})
	// Port 1 refuses the connection, so the probe reports a gateway error
	// rather than a fabricated key.
	if answer.Status != http.StatusBadGateway {
		t.Fatalf("answer = %+v", answer)
	}
}

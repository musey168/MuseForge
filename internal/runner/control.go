package runner

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"agentflow/internal/domain"

	"github.com/gorilla/websocket"
)

const (
	controlReconnectMin = time.Second
	controlReconnectMax = 30 * time.Second
)

// The control channel carries exactly three operations: CLI maintenance, the
// CLI model probe and the SSH host-key probe. The operation names are the
// wire values the platform sends; they are part of the protocol, not of the
// runner's internals.
const (
	controlOperationMaintenance   = "maintenance"
	controlOperationLoginModels   = "login_models"
	controlOperationSSHHostKey    = "ssh_host_key_probe"
)

// controlReadTimeout outlives two server ping periods (30s). Without a read
// deadline a half-open socket keeps ReadJSON blocked forever: the daemon
// looks connected and never reconnects. The server only sends pings, so the
// ping handler (not a pong handler) extends the deadline.
var controlReadTimeout = 75 * time.Second

var controlWriteTimeout = 10 * time.Second

// controlRequest is the request the platform sends over the control channel.
type controlRequest struct {
	Operation     string                          `json:"operation,omitempty"`
	Maintenance   *domain.RunnerMaintenanceRequest `json:"maintenance,omitempty"`
	ModelProvider string                          `json:"modelProvider,omitempty"`
	// SSHHostKeyAddress is the git host whose public key the machine should
	// read. It belongs to the SSH host-key probe operation only.
	SSHHostKeyAddress string `json:"sshHostKeyAddress,omitempty"`
}

// controlResponse is the runner's answer to one control request.
type controlResponse struct {
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
	Code    string          `json:"code,omitempty"`
	Status  int             `json:"status,omitempty"`
}

// controlMessage is the envelope of the runner's outbound control channel:
// the platform asks with a "workspace.request" and reads the answer from the
// matching "workspace.response".
type controlMessage struct {
	Type              string           `json:"type"`
	RequestID         string           `json:"requestId,omitempty"`
	WorkspaceRequest  *controlRequest  `json:"workspaceRequest,omitempty"`
	WorkspaceResponse *controlResponse `json:"workspaceResponse,omitempty"`
}

// controlLoop maintains one outbound WebSocket for immediate server-to-runner
// controls. Reconnect is automatic.
func (d *Daemon) controlLoop(ctx context.Context) {
	backoff := controlReconnectMin
	for ctx.Err() == nil {
		conn, err := d.client.OpenControl(ctx)
		if err != nil {
			log.Printf("control websocket connect: %v", err)
			if errors.Is(err, ErrUnauthorized) {
				if registerErr := d.reregister(ctx); registerErr != nil {
					log.Printf("control re-register: %v", registerErr)
				}
			}
			if !waitContext(ctx, backoff) {
				return
			}
			backoff *= 2
			if backoff > controlReconnectMax {
				backoff = controlReconnectMax
			}
			continue
		}

		d.wsOnline.Store(true)
		backoff = controlReconnectMin
		_ = conn.SetWriteDeadline(time.Now().Add(controlWriteTimeout))
		err = d.readControls(ctx, conn)
		d.wsOnline.Store(false)
		_ = conn.Close()
		if ctx.Err() == nil && err != nil {
			log.Printf("control websocket disconnected: %v", err)
		}
		if !waitContext(ctx, backoff) {
			return
		}
	}
}

// readControls answers the control requests the platform still sends.
func (d *Daemon) readControls(daemonCtx context.Context, conn *websocket.Conn) error {
	ctx, cancel := context.WithCancel(daemonCtx)
	defer cancel()
	var writeMu sync.Mutex
	writeMessage := func(message controlMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(controlWriteTimeout))
		err := conn.WriteJSON(message)
		if err != nil {
			cancel()
		}
		return err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	_ = conn.SetReadDeadline(time.Now().Add(controlReadTimeout))
	conn.SetPingHandler(func(data string) error {
		_ = conn.SetReadDeadline(time.Now().Add(controlReadTimeout))
		err := conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(10*time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		}
		if e, ok := err.(net.Error); ok && e.Timeout() {
			return nil
		}
		return err
	})
	for {
		var message controlMessage
		if err := conn.ReadJSON(&message); err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(controlReadTimeout))
		if message.Type != "workspace.request" || message.WorkspaceRequest == nil {
			continue
		}
		response, startUpdate := d.handleControlRequest(ctx, daemonCtx, *message.WorkspaceRequest)
		err := writeMessage(controlMessage{
			Type:              "workspace.response",
			RequestID:         message.RequestID,
			WorkspaceResponse: &response,
		})
		if startUpdate != nil {
			startUpdate()
		}
		if err != nil {
			_ = conn.Close()
			return err
		}
	}
}

// handleControlRequest serves the two operations the machine inventory still
// offers. The callback starts the actual update after the answer was sent.
func (d *Daemon) handleControlRequest(ctx, daemonCtx context.Context, request controlRequest) (controlResponse, func()) {
	switch request.Operation {
	case controlOperationMaintenance:
		return d.maintenanceResponse(ctx, daemonCtx, request.Maintenance)
	case controlOperationLoginModels:
		response := d.runnerLoginModels(ctx, request.ModelProvider)
		return response, nil
	case controlOperationSSHHostKey:
		response := d.runnerSSHHostKey(ctx, request.SSHHostKeyAddress)
		return response, nil
	default:
		return controlResponse{Status: http.StatusBadRequest, Error: "unsupported runner operation"}, nil
	}
}

// runnerPayload encodes one answer payload for the control channel.
func runnerPayload(value any) controlResponse {
	payload, err := json.Marshal(value)
	if err != nil {
		return controlResponse{Error: err.Error(), Status: http.StatusInternalServerError}
	}
	return controlResponse{Payload: payload, Status: http.StatusOK}
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

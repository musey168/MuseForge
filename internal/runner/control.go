package runner

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"agentflow/internal/domain"

	"github.com/gorilla/websocket"
)

const (
	controlReconnectMin = time.Second
	controlReconnectMax = 30 * time.Second
)

// controlReadTimeout outlives two server ping periods (30s). Without a read
// deadline a half-open socket keeps ReadJSON blocked forever: the daemon
// looks connected and never reconnects. The server only sends pings, so the
// ping handler (not a pong handler) extends the deadline.
var controlReadTimeout = 75 * time.Second

var controlWriteTimeout = 10 * time.Second

// controlMessage is the control-socket wire format. The daemon is an
// inventory: the socket carries no task, no workspace and no conversation —
// only the three operations the machine inventory still needs.
type controlMessage struct {
	Type              string                          `json:"type"`
	RequestID         string                          `json:"requestId,omitempty"`
	WorkspaceRequest  *domain.RunnerWorkspaceRequest  `json:"workspaceRequest,omitempty"`
	WorkspaceResponse *domain.RunnerWorkspaceResponse `json:"workspaceResponse,omitempty"`
}

// controlLoop maintains one outbound WebSocket for the server's control
// operations. Reconnect is automatic.
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
		if err = conn.WriteJSON(controlMessage{Type: "hello"}); err == nil {
			err = d.readControls(ctx, conn)
		}
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

// readControls serves the control requests the machine inventory still needs:
// CLI maintenance, the CLI model probe and the SSH host-key probe. Everything
// else the socket used to carry is gone with the execution engine.
func (d *Daemon) readControls(daemonCtx context.Context, conn *websocket.Conn) error {
	ctx, cancel := context.WithCancel(daemonCtx)
	var requests sync.WaitGroup
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
	slots := make(chan struct{}, 4)
	defer func() { cancel(); _ = conn.Close(); requests.Wait() }()
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
		request := *message.WorkspaceRequest
		switch request.Operation {
		case domain.RunnerWorkspaceOperationMaintenance,
			domain.RunnerWorkspaceOperationLoginModels,
			domain.RunnerWorkspaceOperationSSHHostKeyProbe:
		default:
			_ = writeMessage(controlMessage{
				Type:              "workspace.response",
				RequestID:         message.RequestID,
				WorkspaceResponse: &domain.RunnerWorkspaceResponse{Status: 400, Error: "unsupported control operation"},
			})
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = writeMessage(controlMessage{
				Type:              "workspace.response",
				RequestID:         message.RequestID,
				WorkspaceResponse: &domain.RunnerWorkspaceResponse{Status: 429, Error: "too many control requests"},
			})
			continue
		}
		requests.Add(1)
		go func(requestID string, request domain.RunnerWorkspaceRequest) {
			defer requests.Done()
			defer func() { <-slots }()
			var response domain.RunnerWorkspaceResponse
			var startUpdate func()
			switch request.Operation {
			case domain.RunnerWorkspaceOperationMaintenance:
				response, startUpdate = d.maintenanceResponse(ctx, ctx, request.Maintenance)
			case domain.RunnerWorkspaceOperationLoginModels:
				response = d.runnerLoginModels(ctx, request.ModelProvider)
			default:
				response = d.runnerSSHHostKey(ctx, request)
			}
			err := writeMessage(controlMessage{
				Type:              "workspace.response",
				RequestID:         requestID,
				WorkspaceResponse: &response,
			})
			if startUpdate != nil {
				startUpdate()
			}
			if err != nil {
				_ = conn.Close()
			}
		}(message.RequestID, request)
	}
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

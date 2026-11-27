package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"agentflow/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	runnerWSPingInterval = 30 * time.Second
	runnerWSWriteTimeout = 10 * time.Second
)

// runnerWSReadTimeout outlives two ping periods. Without a read deadline a
// half-open socket (dropped by a middlebox without a FIN) stays registered and
// the runner looks online until the kernel gives up.
var runnerWSReadTimeout = 75 * time.Second

// Reconnect tolerance for transient control RPCs (CLI maintenance, model
// probe, SSH host key probe).
var (
	runnerReconnectWait  = 5 * time.Second
	runnerReconnectGrace = 15 * time.Second
	runnerStartupGrace   = 30 * time.Second
)

type runnerControlMessage struct {
	Type              string                          `json:"type"`
	RequestID         string                          `json:"requestId,omitempty"`
	WorkspaceRequest  *domain.RunnerWorkspaceRequest  `json:"workspaceRequest,omitempty"`
	WorkspaceResponse *domain.RunnerWorkspaceResponse `json:"workspaceResponse,omitempty"`
	ActiveTaskIDs     []string                        `json:"activeTaskIds,omitempty"`
}

type runnerControlHub struct {
	mu      sync.Mutex
	conns   map[string]*runnerControlConn
	pending map[string]runnerWorkspaceWaiter
	nextID  uint64
	// registered is closed and replaced whenever a runner (re)connects.
	registered chan struct{}
	// dropped records when each runner's socket last went away.
	dropped   map[string]time.Time
	startedAt time.Time

	reconnectWait  time.Duration
	reconnectGrace time.Duration
	startupGrace   time.Duration
}

type runnerWorkspaceWaiter struct {
	conn   *runnerControlConn
	result chan domain.RunnerWorkspaceResponse
}

type runnerControlConn struct {
	runnerID                string
	ownerUserID             int64
	supportsSSHHostKeyProbe bool
	ws                      *websocket.Conn
	send                    chan runnerControlMessage
	done                    chan struct{}
	once                    sync.Once
}

func newRunnerControlHub() *runnerControlHub {
	return &runnerControlHub{
		conns:          map[string]*runnerControlConn{},
		pending:        map[string]runnerWorkspaceWaiter{},
		registered:     make(chan struct{}),
		dropped:        map[string]time.Time{},
		startedAt:      time.Now(),
		reconnectWait:  runnerReconnectWait,
		reconnectGrace: runnerReconnectGrace,
		startupGrace:   runnerStartupGrace,
	}
}

var errRunnerWorkspaceUnavailable = errors.New("runner workspace unavailable")

// requestWorkspace performs a transient RPC over the runner's outbound control
// socket. The machine inventory uses it for CLI maintenance, the CLI model
// probe and the SSH host key probe.
func (h *runnerControlHub) requestWorkspace(ctx context.Context, runnerID string, request domain.RunnerWorkspaceRequest) (domain.RunnerWorkspaceResponse, error) {
	if runnerID == "" {
		return domain.RunnerWorkspaceResponse{}, errRunnerWorkspaceUnavailable
	}
	var waitUntil time.Time
	for attempt := 0; attempt < 4; attempt++ {
		conn, err := h.connFor(ctx, runnerID, attempt > 0, &waitUntil)
		if err != nil {
			return domain.RunnerWorkspaceResponse{}, err
		}
		response, sent, err := h.send(ctx, conn, request)
		if sent || !errors.Is(err, errRunnerWorkspaceUnavailable) {
			return response, err
		}
	}
	return domain.RunnerWorkspaceResponse{}, errRunnerWorkspaceUnavailable
}

// connFor returns a live socket for runnerID or errRunnerWorkspaceUnavailable
// once no reconnect is expected in time.
func (h *runnerControlHub) connFor(ctx context.Context, runnerID string, expectReconnect bool, waitUntil *time.Time) (*runnerControlConn, error) {
	for {
		h.mu.Lock()
		conn := h.conns[runnerID]
		if conn != nil && !conn.closed() {
			h.mu.Unlock()
			return conn, nil
		}
		now := time.Now()
		if waitUntil.IsZero() {
			if !expectReconnect && !h.reconnectExpectedLocked(runnerID, now) {
				h.mu.Unlock()
				return nil, errRunnerWorkspaceUnavailable
			}
			*waitUntil = now.Add(h.reconnectWait)
		}
		remaining := waitUntil.Sub(now)
		registered := h.registered
		h.mu.Unlock()
		if remaining <= 0 {
			return nil, errRunnerWorkspaceUnavailable
		}
		timer := time.NewTimer(remaining)
		select {
		case <-registered:
			timer.Stop()
		case <-timer.C:
			return nil, errRunnerWorkspaceUnavailable
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("%w: %w", errRunnerWorkspaceUnavailable, ctx.Err())
		}
	}
}

func (h *runnerControlHub) reconnectExpectedLocked(runnerID string, now time.Time) bool {
	if now.Sub(h.startedAt) < h.startupGrace {
		return true
	}
	dropped, ok := h.dropped[runnerID]
	return ok && now.Sub(dropped) < h.reconnectGrace
}

// send delivers one request; sent reports whether it reached the socket's
// queue, after which it must not be retried elsewhere.
func (h *runnerControlHub) send(ctx context.Context, conn *runnerControlConn, request domain.RunnerWorkspaceRequest) (domain.RunnerWorkspaceResponse, bool, error) {
	if request.Operation == domain.RunnerWorkspaceOperationSSHHostKeyProbe {
		// Runner IDs can be rebound before an old connection disconnects.
		if request.SSHHostKeyOwnerID <= 0 || request.SSHHostKeyOwnerID != conn.ownerUserID {
			return domain.RunnerWorkspaceResponse{
				Status: http.StatusConflict, Error: "执行机器绑定已变更，请刷新后重试",
			}, false, nil
		}
		if !conn.supportsSSHHostKeyProbe {
			return domain.RunnerWorkspaceResponse{
				Status: http.StatusConflict, Error: "该执行机器暂不支持读取 SSH 主机公钥，请升级 Runner 或手动录入",
			}, false, nil
		}
		request.SSHHostKeyOwnerID = 0
	}
	h.mu.Lock()
	h.nextID++
	requestID := fmt.Sprintf("workspace-%d", h.nextID)
	result := make(chan domain.RunnerWorkspaceResponse, 1)
	h.pending[requestID] = runnerWorkspaceWaiter{conn: conn, result: result}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, requestID)
		h.mu.Unlock()
	}()

	select {
	case <-conn.done:
		return domain.RunnerWorkspaceResponse{}, false, errRunnerWorkspaceUnavailable
	default:
	}
	select {
	case conn.send <- runnerControlMessage{Type: "workspace.request", RequestID: requestID, WorkspaceRequest: &request}:
	case <-conn.done:
		return domain.RunnerWorkspaceResponse{}, false, errRunnerWorkspaceUnavailable
	case <-ctx.Done():
		return domain.RunnerWorkspaceResponse{}, false, ctx.Err()
	}
	select {
	case response := <-result:
		return response, true, nil
	case <-conn.done:
		return domain.RunnerWorkspaceResponse{}, true, errRunnerWorkspaceUnavailable
	case <-ctx.Done():
		return domain.RunnerWorkspaceResponse{}, true, ctx.Err()
	}
}

func (h *runnerControlHub) resolveWorkspace(conn *runnerControlConn, requestID string, response domain.RunnerWorkspaceResponse) {
	h.mu.Lock()
	waiter, ok := h.pending[requestID]
	h.mu.Unlock()
	if !ok || waiter.conn != conn {
		return
	}
	select {
	case waiter.result <- response:
	default:
	}
}

func (h *runnerControlHub) register(runner domain.Runner, ws *websocket.Conn, supportsSSHHostKeyProbe bool) *runnerControlConn {
	c := &runnerControlConn{
		runnerID: runner.ID, ownerUserID: runner.OwnerUserID, ws: ws,
		supportsSSHHostKeyProbe: supportsSSHHostKeyProbe,
		send:                    make(chan runnerControlMessage, 64), done: make(chan struct{}),
	}
	h.mu.Lock()
	old := h.conns[runner.ID]
	h.conns[runner.ID] = c
	close(h.registered)
	h.registered = make(chan struct{})
	h.mu.Unlock()
	if old != nil {
		old.close()
	}
	return c
}

func (h *runnerControlHub) unregister(c *runnerControlConn) {
	h.mu.Lock()
	if h.conns[c.runnerID] == c {
		delete(h.conns, c.runnerID)
		h.dropped[c.runnerID] = time.Now()
	}
	h.mu.Unlock()
	c.close()
}

// closeAll drops every runner socket so runners reconnect.
func (h *runnerControlHub) closeAll() {
	h.mu.Lock()
	conns := make([]*runnerControlConn, 0, len(h.conns))
	now := time.Now()
	for id, c := range h.conns {
		conns = append(conns, c)
		h.dropped[id] = now
	}
	h.mu.Unlock()
	for _, c := range conns {
		c.close()
	}
}

func (c *runnerControlConn) close() {
	c.once.Do(func() {
		close(c.done)
		if c.ws != nil {
			_ = c.ws.Close()
		}
	})
}

func (c *runnerControlConn) closed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *runnerControlConn) writeLoop() {
	ticker := time.NewTicker(runnerWSPingInterval)
	defer ticker.Stop()
	for {
		select {
		case message := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(runnerWSWriteTimeout))
			if err := c.ws.WriteJSON(message); err != nil {
				c.close()
				return
			}
		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(runnerWSWriteTimeout))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.close()
				return
			}
		case <-c.done:
			return
		}
	}
}

var runnerWSUpgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
}

// runnerControlConnect upgrades the outbound runner socket and serves the
// control messages the machine inventory needs.
func (h *runnersHandlers) runnerControlConnect(c *gin.Context) {
	runner, ok := h.runnerSessionAuth(c)
	if !ok {
		return
	}
	ws, err := runnerWSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	conn := h.runnerControls.register(runner, ws, c.GetHeader(domain.RunnerSSHHostKeyProbeHeader) == "1")
	defer h.runnerControls.unregister(conn)
	go conn.writeLoop()
	ws.SetReadLimit(20 << 20)
	_ = ws.SetReadDeadline(time.Now().Add(runnerWSReadTimeout))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(runnerWSReadTimeout))
	})
	for {
		var input runnerControlMessage
		if err := ws.ReadJSON(&input); err != nil {
			return
		}
		_ = ws.SetReadDeadline(time.Now().Add(runnerWSReadTimeout))
		if input.Type == "workspace.response" && input.WorkspaceResponse != nil {
			h.runnerControls.resolveWorkspace(conn, input.RequestID, *input.WorkspaceResponse)
		}
	}
}

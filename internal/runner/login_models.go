package runner

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"agentflow/internal/agentcli"
	"agentflow/internal/domain"
	"agentflow/internal/gitwork"
)

// runnerLoginModels answers the model probe: which models the machine's CLI
// can reach with the account it is logged in with.
func (d *Daemon) runnerLoginModels(ctx context.Context, provider string) domain.RunnerWorkspaceResponse {
	for _, cli := range d.currentRuntimes() {
		if cli.Provider != provider || cli.BinaryPath == "" || cli.Status != domain.RuntimeStatusOnline {
			continue
		}
		models, err := agentcli.DiscoverLoginModels(ctx, provider, cli.BinaryPath)
		if err != nil {
			return domain.RunnerWorkspaceResponse{Status: http.StatusBadGateway, Error: err.Error()}
		}
		return runnerWorkspacePayload(models)
	}
	return domain.RunnerWorkspaceResponse{Status: http.StatusNotFound, Error: "执行机器上的 CLI 当前不可用"}
}

// runnerSSHHostKey answers the SSH host-key probe: the public key the machine
// sees for a git host, so personal settings can trust it.
func (d *Daemon) runnerSSHHostKey(ctx context.Context, request domain.RunnerWorkspaceRequest) domain.RunnerWorkspaceResponse {
	dialer := &net.Dialer{}
	key, err := gitwork.ProbeSSHHostPublicKey(ctx, request.SSHHostKeyAddress, dialer.DialContext)
	if err != nil {
		return domain.RunnerWorkspaceResponse{Status: http.StatusBadGateway, Error: err.Error()}
	}
	return runnerWorkspacePayload(key)
}

// runnerWorkspacePayload marshals a value into the control response.
func runnerWorkspacePayload(value any) domain.RunnerWorkspaceResponse {
	payload, err := json.Marshal(value)
	if err != nil {
		return domain.RunnerWorkspaceResponse{Error: err.Error(), Status: http.StatusInternalServerError}
	}
	return domain.RunnerWorkspaceResponse{Payload: payload, Status: http.StatusOK}
}

package runner

import (
	"context"
	"net"
	"net/http"

	"agentflow/internal/agentcli"
	"agentflow/internal/domain"
	"agentflow/internal/gitwork"
)

// runnerLoginModels asks the CLI installed on this machine which models its
// signed-in account can reach. The probe never talks to the control plane and
// never returns the credential it stages.
func (d *Daemon) runnerLoginModels(ctx context.Context, provider string) controlResponse {
	for _, cli := range d.currentRuntimes() {
		if cli.Provider != provider || cli.BinaryPath == "" || cli.Status != domain.RuntimeStatusOnline {
			continue
		}
		models, err := agentcli.DiscoverLoginModels(ctx, provider, cli.BinaryPath)
		if err != nil {
			return controlResponse{Status: http.StatusBadGateway, Error: err.Error()}
		}
		return runnerPayload(models)
	}
	return controlResponse{Status: http.StatusNotFound, Error: "执行机器上的 CLI 当前不可用"}
}

// runnerSSHHostKey answers the SSH host-key probe: the public key the machine
// sees for a git host, so the platform can trust it.
func (d *Daemon) runnerSSHHostKey(ctx context.Context, address string) controlResponse {
	dialer := &net.Dialer{}
	key, err := gitwork.ProbeSSHHostPublicKey(ctx, address, dialer.DialContext)
	if err != nil {
		return controlResponse{Status: http.StatusBadGateway, Error: err.Error()}
	}
	return runnerPayload(key)
}

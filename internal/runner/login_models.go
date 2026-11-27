package runner

import (
	"context"
	"net/http"

	"agentflow/internal/agentcli"
	"agentflow/internal/domain"
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

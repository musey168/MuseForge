package runner

import (
	"context"

	"agentflow/internal/domain"
	"agentflow/internal/managedcli"
)

// detectManagedRuntimes reports the CLIs a managed runner has installed. The
// managed CLI root coordinates the installation, so the discovery runs under
// its usage lock and the reported versions are the real ones.
func (d *Daemon) detectManagedRuntimes(ctx context.Context) ([]domain.RunnerRuntime, error) {
	lease, err := managedcli.BeginClaim(ctx, d.cfg.ManagedCLIRoot)
	if err != nil {
		return nil, err
	}
	defer lease.Close()
	inventory, err := managedcli.ReadInventory(d.cfg.ManagedCLIRoot)
	if err != nil {
		return nil, err
	}
	// Retain usage during --version so activation cannot race a probe.
	return detectRuntimes(ctx, d.cfg.ExtraBinaries, inventory), nil
}

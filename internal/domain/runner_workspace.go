package domain

import (
	"encoding/json"
)

// The control channel between the platform and a machine carries exactly
// three operations: CLI maintenance, the CLI model probe and the SSH host-key
// probe. The operation names are wire values, not of the runner's internals.
const (
	RunnerWorkspaceOperationMaintenance     = "maintenance"
	RunnerWorkspaceOperationLoginModels     = "login_models"
	RunnerWorkspaceOperationSSHHostKeyProbe = "ssh_host_key_probe"
)

// RunnerSSHKnownHostsWireLimit is the encoded allowance for the optional
// known_hosts field in a runner task claim.
const RunnerSSHKnownHostsWireLimit = 1 << 20

// RunnerSSHHostKeyProbeHeader advertises host key discovery on the authenticated
// control connection. Older runners can still receive and use known_hosts pins.
const RunnerSSHHostKeyProbeHeader = "X-AgentFlow-SSH-Host-Key-Probe"

// GitSSHKnownHosts carries the platform's scoped SSH host keys over an
// authenticated runner transport. Configuration errors are also sent so the
// runner can fail closed when the platform cannot supply the configured pins.
type GitSSHKnownHosts struct {
	Entries    string `json:"entries,omitempty"`
	ReadFailed bool   `json:"readFailed,omitempty"`
	TooLarge   bool   `json:"tooLarge,omitempty"`
}

// RunnerWorkspaceRequest is a bounded control request sent over the runner's
// outbound control socket. Responses are transient and are never persisted.
type RunnerWorkspaceRequest struct {
	Maintenance   *RunnerMaintenanceRequest `json:"maintenance,omitempty"`
	ModelProvider string                    `json:"modelProvider,omitempty"`
	// SSHHostKeyAddress is the git host whose public key the machine should
	// read. It belongs to the SSH host-key probe operation only.
	SSHHostKeyAddress string `json:"sshHostKeyAddress,omitempty"`
	// SSHHostKeyOwnerID accompanies probes across platform handover. The
	// control hub checks it against the socket and clears it before sending.
	SSHHostKeyOwnerID int64  `json:"sshHostKeyOwnerId,omitempty"`
	Operation         string `json:"operation"`
}

// RunnerMaintenanceRequest asks the machine to check or apply a CLI update.
type RunnerMaintenanceRequest struct {
	Provider   string `json:"provider,omitempty"` // Empty selects the Runner binary.
	Apply      bool   `json:"apply,omitempty"`
	StatusOnly bool   `json:"statusOnly,omitempty"`
}

// RunnerMaintenanceResult is the machine's answer to a maintenance request.
type RunnerMaintenanceResult struct {
	Status    string `json:"status"`
	Current   string `json:"current,omitempty"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message,omitempty"`
	CanUpdate bool   `json:"canUpdate,omitempty"`
}

// RunnerWorkspaceResponse is the machine's answer to one control request.
type RunnerWorkspaceResponse struct {
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
	Code    string          `json:"code,omitempty"`
	Status  int             `json:"status,omitempty"`
}

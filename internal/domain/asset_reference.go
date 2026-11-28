package domain

// An asset reference names one platform object a scheduled task or a
// conversation selected: its kind, its identifier and enough context to
// show it without another lookup.
type AssetReference struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
}

// CloneAssetReferences returns an independent copy for persisted task
// payloads and message construction.
func CloneAssetReferences(refs []AssetReference) []AssetReference {
	if len(refs) == 0 {
		return nil
	}
	out := make([]AssetReference, len(refs))
	copy(out, refs)
	return out
}

// ExecutorDefaults records the executor, model and effort a run inherits
// from the account that owns it.
type ExecutorDefaults struct {
	Executor string `json:"executor,omitempty"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
}

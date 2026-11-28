package domain

import "testing"

func TestAgentCLIRuntimeAuthModeIsPersonal(t *testing.T) {
	r := Runner{Runtimes: []RunnerRuntime{
		{Provider: "opencode", AuthMode: RuntimeAuthLocal},
		{Provider: "codex", AuthMode: RuntimeAuthLocal},
	}}
	if r.RuntimeAuthMode("opencode") != RuntimeAuthPersonal {
		t.Fatal("agent CLI must use personal credentials even when a local mode is stored")
	}
	if r.RuntimeAuthMode("codex") != RuntimeAuthLocal {
		t.Fatal("codex local login must stay local")
	}
	if r.RuntimeAuthMode("codebuddy") != RuntimeAuthPersonal {
		t.Fatal("codebuddy still defaults to personal credentials")
	}
	if r.RuntimeAuthMode("agy") != RuntimeAuthLocal {
		t.Fatal("Antigravity supports local account login")
	}
	r.Runtimes = append(r.Runtimes, RunnerRuntime{Provider: "agy", AuthMode: RuntimeAuthPersonal})
	if r.RuntimeAuthMode("agy") != RuntimeAuthPersonal {
		t.Fatal("Antigravity API-key preference was ignored")
	}
}

func TestRunnerSupportsCLISettings(t *testing.T) {
	for version, want := range map[string]bool{
		"": false, "dev": false, "0.10.1": false, "0.10.2": true, "v0.10.2": true, "0.11.0": true, "1.0.0": true,
	} {
		if got := (Runner{RunnerVersion: version}).SupportsCLISettings(); got != want {
			t.Errorf("SupportsCLISettings(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestRunnerSupportsProvider(t *testing.T) {
	r := Runner{Runtimes: []RunnerRuntime{
		{Provider: "codex", Status: RuntimeStatusOnline},
		{Provider: "agy", Status: RuntimeStatusDisabled},
	}}
	if !r.SupportsProvider("codex") || !r.SupportsProvider("CODEX") {
		t.Fatal("a detected CLI must be supported")
	}
	if r.SupportsProvider("agy") {
		t.Fatal("a disabled CLI must not be supported")
	}
	if r.SupportsProvider("") {
		t.Fatal("an empty provider must not be supported")
	}
}

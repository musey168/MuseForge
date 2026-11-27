package runner

import (
	"context"
	"testing"
)

func TestRuntimeProviderSpecsDefaultsOnly(t *testing.T) {
	specs := runtimeProviderSpecs(nil)
	if len(specs) != len(defaultProviders) {
		t.Fatalf("len(specs) = %d, want %d", len(specs), len(defaultProviders))
	}
	for i, spec := range defaultProviders {
		if specs[i].Kind != spec.Kind || specs[i].Binary != spec.Binary {
			t.Fatalf("specs[%d] = %+v, want %+v", i, specs[i], spec)
		}
	}
}

func TestRuntimeProviderSpecsAppendsSortedExtraKinds(t *testing.T) {
	specs := runtimeProviderSpecs(map[string]string{
		"zeta":        "/opt/zeta",
		"alpha":       "/opt/alpha",
		"claude-code": "/opt/claude",
	})
	if len(specs) != len(defaultProviders)+2 {
		t.Fatalf("len(specs) = %d, want %d", len(specs), len(defaultProviders)+2)
	}
	for i, spec := range defaultProviders {
		// An extra binary for a default kind is an override applied later,
		// not a replacement of the default spec.
		if specs[i].Kind != spec.Kind || specs[i].Binary != spec.Binary {
			t.Fatalf("specs[%d] = %+v, want %+v", i, specs[i], spec)
		}
	}
	extra := specs[len(defaultProviders):]
	want := []providerSpec{{Kind: "alpha", Binary: "/opt/alpha"}, {Kind: "zeta", Binary: "/opt/zeta"}}
	for i := range want {
		if extra[i].Kind != want[i].Kind || extra[i].Binary != want[i].Binary {
			t.Fatalf("extra[%d] = %+v, want %+v", i, extra[i], want[i])
		}
	}
}

// The runner reports the CLIs installed on the machine and nothing else:
// there are no in-process pseudo-runtimes left.
func TestDetectRuntimesReportsOnlyInstalledCLIs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	got := detectRuntimes(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("detectRuntimes = %+v, want no runtime on an empty machine", got)
	}
	for _, rt := range got {
		if rt.BinaryPath == "(builtin)" {
			t.Fatalf("detectRuntimes reported a builtin runtime: %+v", rt)
		}
	}
}

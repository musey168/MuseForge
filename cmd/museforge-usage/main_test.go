package main

import (
	"strings"
	"testing"
)

func TestThousands(t *testing.T) {
	cases := map[int64]string{
		0:          "0",
		999:        "999",
		1000:       "1,000",
		1234567:    "1,234,567",
		-2500000:   "-2,500,000",
		1234567890: "1,234,567,890",
	}
	for value, want := range cases {
		if got := thousands(value); got != want {
			t.Fatalf("thousands(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestRunRejectsUnknownAgent(t *testing.T) {
	err := run([]string{"sync", "--agent", "cursor", "--dry-run"})
	if err == nil || !strings.Contains(err.Error(), "unknown --agent") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRequiresTokenWithoutDryRun(t *testing.T) {
	err := run([]string{"sync", "--agent", "omp"})
	if err == nil || !strings.Contains(err.Error(), "--token") {
		t.Fatalf("err = %v", err)
	}
}

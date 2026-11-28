//go:build clilive

package httpapi

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

)

var liveCLIOrder = []string{"codebuddy", "codex", "claude-code", "opencode", "cursor", "agy", "kimi", "pi", "dsh", "zcode"}

type liveCLIConfig struct {
	Key         string `json:"key"`
	BaseURL     string `json:"baseUrl"`
	Protocol    string `json:"protocol"`
	Model       string `json:"model"`
	Environment string `json:"environment"`
}

func liveTimeout() time.Duration {
	seconds, _ := strconv.Atoi(os.Getenv("CLI_LIVE_TIMEOUT"))
	if seconds <= 0 {
		seconds = 90
	}
	return time.Duration(seconds) * time.Second
}

func loadLiveConfig(t *testing.T) map[string]liveCLIConfig {
	t.Helper()
	path := os.Getenv("CLI_LIVE_CONFIG")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]liveCLIConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}


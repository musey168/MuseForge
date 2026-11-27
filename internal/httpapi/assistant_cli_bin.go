package httpapi

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"agentflow/internal/agentcli"
)

// The personal build no longer runs an agent, so the platform only needs to
// find a CLI binary to probe its model catalog.

var assistantCLIGlobalDirs = []string{"/usr/local/bin", "/usr/bin"}

var errAssistantCLIMissing = errors.New("assistant CLI not installed")

type assistantCLIMissingError struct{ engine, binary string }

func (e assistantCLIMissingError) Error() string {
	return fmt.Sprintf("未安装 %s（%s）", assistantEngineLabel(e.engine), e.binary)
}

func (e assistantCLIMissingError) Is(target error) bool { return target == errAssistantCLIMissing }

type assistantCredentialError struct{ engine string }

func (e assistantCredentialError) Error() string {
	return fmt.Sprintf("%s 拒绝了你的 API Key，请确认 Key 与所选服务一致", assistantEngineLabel(e.engine))
}

func (e assistantCredentialError) Is(target error) bool {
	return target == errAssistantCredentialRejected
}

var errAssistantCredentialRejected = errors.New("assistant credential rejected")

// resolveAssistantEngineBin finds the executable a CLI engine installs.
func resolveAssistantEngineBin(engine string) (string, error) {
	binary := assistantEngineBinary(engine)
	name := strings.ToUpper(strings.ReplaceAll(engine, "-", "_"))
	if override := strings.TrimSpace(os.Getenv("AGENTFLOW_" + name + "_BIN")); override != "" && fileExecutable(override) {
		return override, nil
	}
	if path, ok := agentcli.OnDemandCLIPath(engine); ok {
		return path, nil
	}
	if path, err := exec.LookPath(binary); err == nil {
		return path, nil
	}
	home, _ := os.UserHomeDir()
	if strings.TrimSpace(home) != "" {
		candidates := []string{home + "/.local/bin/" + binary}
		for _, dir := range assistantCLIGlobalDirs {
			candidates = append(candidates, dir+"/"+binary)
		}
		for _, candidate := range candidates {
			if fileExecutable(candidate) {
				return candidate, nil
			}
		}
	}
	return "", assistantCLIMissingError{engine: engine, binary: binary}
}

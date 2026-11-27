package agentcli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// loginSpec says where a CLI keeps its native account login and which file the
// probe has to stage in an isolated HOME.
type loginSpec struct {
	label, dir, file, configEnv string
}

func loginSpecFor(provider string) (loginSpec, bool) {
	switch provider {
	case "codex":
		return loginSpec{"Codex", ".codex", "auth.json", "CODEX_HOME"}, true
	case "claude-code":
		return loginSpec{"Claude Code", ".claude", ".credentials.json", "CLAUDE_CONFIG_DIR"}, true
	case AgentCLIAntigravity:
		return loginSpec{"Antigravity", ".gemini/antigravity-cli", "antigravity-oauth-token", ""}, true
	default:
		return loginSpec{}, false
	}
}

// DiscoverLoginModels asks the CLI installed on this machine which models its
// signed-in account can reach. It never talks to the control plane and never
// returns the credential it stages.
func DiscoverLoginModels(ctx context.Context, provider, binary string) ([]domain.CLIModel, error) {
	spec, ok := loginSpecFor(provider)
	if !ok {
		return nil, fmt.Errorf("该 CLI 暂不支持获取账号模型")
	}
	root, err := os.MkdirTemp("", "museforge-login-models-")
	if err != nil {
		return nil, fmt.Errorf("无法创建模型查询目录")
	}
	defer os.RemoveAll(root)
	home := filepath.Join(root, "home")
	configDir := filepath.Join(home, spec.dir)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("无法准备账号模型查询目录")
	}
	env := map[string]string{"NO_COLOR": "1", "CI": "1"}
	if spec.configEnv != "" {
		env[spec.configEnv] = configDir
	}
	if provider == AgentCLIAntigravity {
		env["SSH_CONNECTION"] = "127.0.0.1 0 127.0.0.1 0"
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	if provider == "claude-code" {
		if token := strings.TrimSpace(os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")); token != "" {
			env["CLAUDE_CODE_OAUTH_TOKEN"] = token
		}
	}
	if err := stageLogin(spec, configDir); err != nil {
		return nil, err
	}
	if provider == AgentCLIAntigravity {
		if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte("{}"), 0o600); err != nil {
			return nil, fmt.Errorf("无法准备 Antigravity 的账号登录配置")
		}
	}
	path := filepath.Dir(binary) + string(os.PathListSeparator) + os.Getenv("PATH")
	env["PATH"] = path
	environment := appendEnvironment(baseEnvironment(home), env)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if provider == AgentCLIAntigravity {
		cmd := exec.CommandContext(ctx, binary, "models")
		cmd.Dir, cmd.Env, cmd.WaitDelay = root, environment, time.Second
		output, err := cmd.Output()
		if err != nil || len(output) > 2<<20 {
			return nil, fmt.Errorf("Antigravity 无法返回账号模型，请检查执行机器上的 CLI 版本和登录状态")
		}
		models := antigravityLoginModels(string(output))
		if len(models) == 0 {
			return nil, fmt.Errorf("Antigravity 未返回可用的账号模型")
		}
		return models, nil
	}
	args := []string{"app-server"}
	if provider == "claude-code" {
		args = []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--setting-sources", ""}
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir, cmd.Env = root, environment
	cmd.WaitDelay = time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("无法连接 %s 查询模型", spec.label)
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("无法读取 %s 的模型列表", spec.label)
	}
	defer output.Close()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("无法启动 %s 查询模型", spec.label)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(io.LimitReader(output, 2<<20))
	scanner.Buffer(make([]byte, 16<<10), 1<<20)
	send := json.NewEncoder(input)
	var models []domain.CLIModel
	if provider == "codex" {
		models = codexLoginModels(send, scanner)
	} else {
		models = claudeLoginModels(send, scanner)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("查询 %s 模型超时", spec.label)
	}
	if models == nil {
		return nil, fmt.Errorf("%s 无法返回账号模型，请检查执行机器上的 CLI 版本和登录状态", spec.label)
	}
	return models, nil
}

// stageLogin copies only the native login file into the probe's isolated HOME.
func stageLogin(spec loginSpec, targetDir string) error {
	source := strings.TrimSpace(os.Getenv(spec.configEnv))
	if source == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			source = filepath.Join(home, spec.dir)
		}
	}
	data, err := readLoginFile(source, spec.file)
	if err != nil {
		return fmt.Errorf("执行机器上的 %s 本机登录态不可用，请检查 Runner 账户的 CLI 登录", spec.label)
	}
	if err := os.WriteFile(filepath.Join(targetDir, spec.file), data, 0o600); err != nil {
		return fmt.Errorf("无法准备 %s 的本机登录态", spec.label)
	}
	return nil
}

func readLoginFile(dir, name string) ([]byte, error) {
	if dir == "" {
		return nil, os.ErrNotExist
	}
	const maxSize = 1 << 20
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSize {
		return nil, fmt.Errorf("invalid login file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize || !json.Valid(data) {
		return nil, fmt.Errorf("invalid login file")
	}
	return data, nil
}

func baseEnvironment(home string) []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "TERM=xterm-256color", "LANG=C.UTF-8"}
}

func appendEnvironment(base []string, overlays ...map[string]string) []string {
	values := map[string]string{}
	order := make([]string, 0, len(base))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = item
	}
	for _, overlay := range overlays {
		keys := make([]string, 0, len(overlay))
		for key := range overlay {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if _, exists := values[key]; !exists {
				order = append(order, key)
			}
			values[key] = key + "=" + overlay[key]
		}
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, values[key])
	}
	return out
}

func antigravityLoginModels(output string) []domain.CLIModel {
	var models []domain.CLIModel
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		id, name, found := strings.Cut(strings.TrimSpace(line), "\t")
		if !found || id == "" || strings.ContainsAny(id, " \r\n") || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, domain.CLIModel{ID: id, Name: strings.TrimSpace(name)})
	}
	return models
}

func codexLoginModels(send *json.Encoder, scanner *bufio.Scanner) []domain.CLIModel {
	if send.Encode(map[string]any{
		"id": 1, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]string{"name": "museforge", "version": "1.0.0"}},
	}) != nil || codexLoginResult(scanner, 1) == nil {
		return nil
	}
	if send.Encode(map[string]string{"method": "initialized"}) != nil ||
		send.Encode(map[string]any{"id": 2, "method": "account/read", "params": map[string]bool{"refreshToken": false}}) != nil {
		return nil
	}
	var account struct {
		Account *struct{} `json:"account"`
	}
	if json.Unmarshal(codexLoginResult(scanner, 2), &account) != nil || account.Account == nil {
		return nil
	}
	if send.Encode(map[string]any{"id": 3, "method": "model/list", "params": map[string]any{"limit": 100, "includeHidden": false}}) != nil {
		return nil
	}
	var result struct {
		Data []struct {
			Model       string `json:"model"`
			DisplayName string `json:"displayName"`
			Efforts     *[]struct {
				Value string `json:"reasoningEffort"`
			} `json:"supportedReasoningEfforts"`
		} `json:"data"`
	}
	if json.Unmarshal(codexLoginResult(scanner, 3), &result) != nil || result.Data == nil {
		return nil
	}
	models := make([]domain.CLIModel, 0, len(result.Data))
	for _, item := range result.Data {
		if id := strings.TrimSpace(item.Model); id != "" {
			model := domain.CLIModel{ID: id, Name: item.DisplayName}
			if item.Efforts != nil {
				levels := make([]string, 0, len(*item.Efforts))
				for _, level := range *item.Efforts {
					levels = append(levels, level.Value)
				}
				model.DiscoveredReasoningEfforts = &levels
			}
			models = append(models, model)
		}
	}
	return models
}

func codexLoginResult(scanner *bufio.Scanner, id int) json.RawMessage {
	for scanner.Scan() {
		var message struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) == nil && message.ID == id {
			return message.Result
		}
	}
	return nil
}

func claudeLoginModels(send *json.Encoder, scanner *bufio.Scanner) []domain.CLIModel {
	const id = "museforge-models"
	if send.Encode(map[string]any{
		"type": "control_request", "request_id": id,
		"request": map[string]string{"subtype": "initialize"},
	}) != nil {
		return nil
	}
	for scanner.Scan() {
		var message struct {
			Type     string `json:"type"`
			Response struct {
				RequestID string `json:"request_id"`
				Subtype   string `json:"subtype"`
				Response  struct {
					Models []struct {
						Value          string    `json:"value"`
						DisplayName    string    `json:"displayName"`
						SupportsEffort *bool     `json:"supportsEffort"`
						EffortLevels   *[]string `json:"supportedEffortLevels"`
					} `json:"models"`
				} `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil || message.Type != "control_response" || message.Response.RequestID != id {
			continue
		}
		if message.Response.Subtype != "success" || message.Response.Response.Models == nil {
			return nil
		}
		models := make([]domain.CLIModel, 0, len(message.Response.Response.Models))
		for _, item := range message.Response.Response.Models {
			if id := strings.TrimSpace(item.Value); id != "" {
				levels := item.EffortLevels
				if item.SupportsEffort != nil && !*item.SupportsEffort {
					levels = &[]string{}
				}
				models = append(models, domain.CLIModel{ID: id, Name: item.DisplayName, DiscoveredReasoningEfforts: levels})
			}
		}
		return models
	}
	return nil
}

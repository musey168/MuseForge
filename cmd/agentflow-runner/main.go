package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"agentflow/internal/runner"
)

// agentflow-runner is the machine inventory daemon (cli-connect.md). It runs
// on a user machine, connects to the AgentFlow control plane over outbound
// HTTPS only, and registers the machine with the Agent CLIs it found. It
// never receives work: no task dispatch, no workspace, no sandbox. The
// credentials of those CLIs stay on this machine.
func main() {
	server := flag.String("server", "", "LoopForge server URL, e.g. https://workflow.example.com (or AGENTFLOW_SERVER_URL)")
	token := flag.String("token", "", "runner bootstrap token (or AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN)")
	name := flag.String("name", "", "runner display name (default: hostname)")
	statePath := flag.String("state", "", "state file path (default: ~/.agentflow-runner/state.json)")
	heartbeat := flag.Duration("heartbeat", 30*time.Second, "heartbeat interval")
	autoUpdate := flag.Bool("auto-update", true, "check for platform updates")
	labels := flag.String("labels", "", "comma-separated runner labels")
	flag.Parse()
	loadDotEnv()

	cfg := runner.Config{
		ServerURL:         firstNonEmpty(*server, os.Getenv("AGENTFLOW_SERVER_URL")),
		BootstrapToken:    firstNonEmpty(*token, os.Getenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN")),
		StatePath:         firstNonEmpty(*statePath, os.Getenv("AGENTFLOW_RUNNER_STATE_PATH")),
		Name:              firstNonEmpty(*name, os.Getenv("AGENTFLOW_RUNNER_NAME")),
		HeartbeatInterval: *heartbeat,
		DisableAutoUpdate: !*autoUpdate,
		ManagedCLIRoot:    strings.TrimSpace(os.Getenv("AGENTFLOW_MANAGED_CLI_ROOT")),
	}
	cfg.Labels = parseLabels(*labels)

	daemon, err := runner.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("agentflow-runner %s connecting to %s", runner.Version(), cfg.ServerURL)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := daemon.Run(ctx); errors.Is(err, runner.ErrUpdateReady) {
		binary, exeErr := os.Executable()
		if exeErr != nil {
			log.Fatal("locate updated runner: ", exeErr)
		}
		log.Printf("restarting updated runner")
		if exeErr := syscall.Exec(binary, os.Args, os.Environ()); exeErr != nil {
			log.Fatal("restart updated runner: ", exeErr)
		}
	} else if err != nil {
		log.Fatal(err)
	}
	log.Printf("runner stopped")
}

// parseLabels splits the comma-separated -labels flag, trimming blanks and
// dropping empty entries. It returns nil when no label remains.
func parseLabels(raw string) []string {
	var labels []string
	for _, label := range strings.Split(raw, ",") {
		if label = strings.TrimSpace(label); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// loadDotEnv mirrors cmd/agentflow so a runner machine can keep its bootstrap
// token in .env next to the working directory.
func loadDotEnv() {
	raw, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

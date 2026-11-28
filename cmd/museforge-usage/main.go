// museforge-usage backfills and follows the local usage stores of coding
// agents into a MuseForge project. One session becomes one execution, so a
// repeated run is a no-op on the platform instead of double counting.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agentflow/internal/agentusage"
	"agentflow/internal/domain"
)

const defaultURL = "http://127.0.0.1:8090"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "museforge-usage:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		return errors.New("missing subcommand")
	}
	switch args[0] {
	case "sync":
		return runSync(args[1:])
	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func usage(out *os.File) {
	fmt.Fprint(out, `museforge-usage sync --agent omp|codex|pi|claude [options]

Reads the local usage store of one coding agent and reports one execution per
session to POST /api/insights/v1/ingest. Re-running is safe: the platform
recognises a session it already has.

Options:
  --agent string     omp, codex, pi or claude (required)
  --url string       platform base URL (default `+defaultURL+`)
  --token string     data-source Bearer token (required unless --dry-run)
  --since duration   only sessions finished within this window, e.g. 720h
                     (default 0: backfill all history)
  --interval duration repeat every interval until interrupted, e.g. 5m
  --dry-run          print what would be sent and send nothing
  --root string      override the agent data directory
                     (omp: ~/.omp, codex: ~/.codex, pi: ~/.pi, claude: ~/.claude)
`)
}

type options struct {
	agent    agentusage.Agent
	url      string
	token    string
	since    time.Duration
	interval time.Duration
	dryRun   bool
	root     string
}

func runSync(args []string) error {
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var (
		agentName string
		opts      options
	)
	flags.StringVar(&agentName, "agent", "", "usage source: omp, codex, pi or claude")
	flags.StringVar(&opts.url, "url", defaultURL, "platform base URL")
	flags.StringVar(&opts.token, "token", "", "data-source Bearer token")
	flags.DurationVar(&opts.since, "since", 0, "only sync sessions finished within this window (0 = all history)")
	flags.DurationVar(&opts.interval, "interval", 0, "repeat every interval until interrupted (0 = once)")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "print what would be sent without sending")
	flags.StringVar(&opts.root, "root", "", "data directory override")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	agent, ok := agentusage.ParseAgent(agentName)
	if !ok {
		return fmt.Errorf("unknown --agent %q (want omp, codex, pi or claude)", agentName)
	}
	opts.agent = agent
	opts.url = strings.TrimSpace(opts.url)
	opts.token = strings.TrimSpace(opts.token)
	if !opts.dryRun {
		if opts.token == "" {
			return errors.New("--token is required unless --dry-run")
		}
		if opts.url == "" {
			return errors.New("--url is required unless --dry-run")
		}
	}
	if opts.since < 0 || opts.interval < 0 {
		return errors.New("--since and --interval cannot be negative")
	}

	reader := agentusage.New(agent, opts.root)
	var client *agentusage.Client
	if !opts.dryRun {
		client = agentusage.NewClient(opts.url, opts.token)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		err := syncOnce(ctx, reader, client, opts)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			if opts.interval == 0 {
				return err
			}
			var apiErr *agentusage.APIError
			if errors.As(err, &apiErr) && apiErr.Fatal() {
				return err
			}
			fmt.Fprintln(os.Stderr, "museforge-usage:", err)
		}
		if opts.interval == 0 || opts.dryRun {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(opts.interval):
		}
	}
}

func syncOnce(ctx context.Context, reader agentusage.Reader, client *agentusage.Client, opts options) error {
	if opts.interval > 0 && !opts.dryRun {
		fmt.Printf("--- %s: %s sync ---\n", time.Now().UTC().Format(time.RFC3339), reader.Agent())
	}
	sessions, err := reader.Read(ctx)
	if err != nil {
		return err
	}
	var cutoff time.Time
	if opts.since > 0 {
		cutoff = time.Now().Add(-opts.since)
	}
	sessions = agentusage.FilterSince(sessions, cutoff)
	executions := agentusage.Executions(reader.Agent(), sessions)
	if opts.dryRun {
		printDryRun(reader, executions, cutoff)
		return nil
	}
	return send(ctx, client, executions)
}

func send(ctx context.Context, client *agentusage.Client, executions []domain.InsightExecution) error {
	var accepted, duplicates, conflicts int
	var failure error
	for _, execution := range executions {
		result, err := client.Ingest(ctx, execution)
		if err != nil {
			var apiErr *agentusage.APIError
			if errors.As(err, &apiErr) && apiErr.Conflict() {
				conflicts++
				fmt.Printf("skipped-conflict %s: %s\n", execution.ExternalExecutionID, apiErr.Message)
				continue
			}
			if ctx.Err() != nil {
				failure = ctx.Err()
			} else {
				failure = fmt.Errorf("%s: %w", execution.ExternalExecutionID, err)
			}
			break
		}
		switch result {
		case agentusage.ResultAccepted:
			accepted++
			fmt.Printf("accepted %s\n", execution.ExternalExecutionID)
		default:
			duplicates++
		}
	}
	fmt.Printf("summary: accepted=%d duplicate=%d skipped-conflict=%d\n", accepted, duplicates, conflicts)
	return failure
}

func printDryRun(reader agentusage.Reader, executions []domain.InsightExecution, cutoff time.Time) {
	fmt.Printf("%s (%s)\n", reader.Agent(), reader.Root())
	if cutoff.IsZero() {
		fmt.Println("window: all history")
	} else {
		fmt.Printf("window: finished since %s\n", cutoff.UTC().Format(time.RFC3339))
	}
	fmt.Printf("sessions: %d\n", len(executions))
	for _, execution := range executions {
		input, output, cacheRead, cacheWrite, reasoning := tokenTotals(execution.Usage)
		fmt.Printf("  %s  %s  input=%s cache_read=%s cache_write=%s output=%s reasoning=%s cost=%s  %s\n",
			execution.StartedAt.Format(time.RFC3339),
			execution.FinishedAt.Sub(execution.StartedAt).Round(time.Second),
			thousands(input), thousands(cacheRead), thousands(cacheWrite),
			thousands(output), thousands(reasoning), costText(execution.Usage),
			execution.ExternalExecutionID,
		)
	}
	input, output, cacheRead, cacheWrite, reasoning := tokenTotals(allUsage(executions))
	fmt.Printf("totals: sessions=%d input=%s (cache_read=%s cache_write=%s) output=%s reasoning=%s cost=%s\n",
		len(executions), thousands(input), thousands(cacheRead), thousands(cacheWrite),
		thousands(output), thousands(reasoning), costText(allUsage(executions)),
	)
	fmt.Println("dry run: nothing sent")
}

func allUsage(executions []domain.InsightExecution) []domain.InsightUsage {
	var usage []domain.InsightUsage
	for _, execution := range executions {
		usage = append(usage, execution.Usage...)
	}
	return usage
}

func tokenTotals(usage []domain.InsightUsage) (input, output, cacheRead, cacheWrite, reasoning int64) {
	for _, entry := range usage {
		input += entry.InputTokens
		output += entry.OutputTokens
		cacheRead += entry.CacheReadTokens
		cacheWrite += entry.CacheWriteTokens
		reasoning += entry.ReasoningTokens
	}
	return input, output, cacheRead, cacheWrite, reasoning
}

// costText prints the summed reported cost, or "unknown" when any entry has
// no recorded cost.
func costText(usage []domain.InsightUsage) string {
	var total float64
	for _, entry := range usage {
		if entry.CostUSD == nil || (entry.CostStatus != "reported" && entry.CostStatus != "estimated") {
			return "unknown"
		}
		total += *entry.CostUSD
	}
	return fmt.Sprintf("$%.4f", total)
}

func thousands(value int64) string {
	digits := strconv.FormatInt(value, 10)
	negative := strings.HasPrefix(digits, "-")
	if negative {
		digits = digits[1:]
	}
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	if negative {
		return "-" + digits
	}
	return digits
}

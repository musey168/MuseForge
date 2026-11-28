// Package agentusage reads the local usage stores of coding agents (omp, Codex,
// pi, Claude Code) and turns one session into one platform execution.
//
// It reads counters, model names, providers and timestamps only. Prompt and
// message content is never inspected or forwarded.
package agentusage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// Agent identifies one supported usage source. The value is also the platform
// workflow ID, so it must stay stable across releases.
type Agent string

const (
	AgentOMP    Agent = "omp"
	AgentCodex  Agent = "codex"
	AgentPi     Agent = "pi"
	AgentClaude Agent = "claude"
)

// Agents lists the sources in the order the CLI documents them.
var Agents = []Agent{AgentOMP, AgentCodex, AgentPi, AgentClaude}

// DisplayName is the human-readable workflow name reported to the platform.
func (a Agent) DisplayName() string {
	switch a {
	case AgentOMP:
		return "omp"
	case AgentCodex:
		return "Codex"
	case AgentPi:
		return "pi"
	case AgentClaude:
		return "Claude Code"
	}
	return string(a)
}

// ParseAgent resolves the --agent flag.
func ParseAgent(value string) (Agent, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, agent := range Agents {
		if string(agent) == normalized {
			return agent, true
		}
	}
	if normalized == "claude-code" {
		return AgentClaude, true
	}
	return "", false
}

// Session is one agent session already reduced to platform usage semantics:
// InputTokens is inclusive of the cache counts.
type Session struct {
	Key        string
	StartedAt  time.Time
	FinishedAt time.Time
	// Status is the terminal state; empty means succeeded.
	Status string
	// ExecutionDurationMs is the sum of the recorded step durations: message
	// durations, not wall-clock time, so a paused session is not inflated.
	ExecutionDurationMs *int64
	Usage               []domain.InsightUsage
}

// Reader reads the completed sessions of one agent.
type Reader interface {
	Agent() Agent
	// Root reports where the reader looks for data; shown in diagnostics.
	Root() string
	Read(ctx context.Context) ([]Session, error)
}

// New builds the reader for an agent. root overrides the source's default data
// directory; an empty root means the documented default.
func New(agent Agent, root string) Reader {
	switch agent {
	case AgentOMP:
		return NewOMP(root)
	case AgentCodex:
		return NewCodex(root)
	case AgentPi:
		return NewPi(root)
	case AgentClaude:
		return NewClaude(root)
	}
	return nil
}

// FilterSince keeps the sessions that finished at or after cutoff. A zero
// cutoff keeps everything, which is what the first run needs to backfill.
func FilterSince(sessions []Session, cutoff time.Time) []Session {
	if cutoff.IsZero() {
		return sessions
	}
	out := make([]Session, 0, len(sessions))
	for _, session := range sessions {
		if !session.FinishedAt.Before(cutoff) {
			out = append(out, session)
		}
	}
	return out
}

func sortSessions(sessions []Session) {
	sort.Slice(sessions, func(i, j int) bool {
		if !sessions[i].StartedAt.Equal(sessions[j].StartedAt) {
			return sessions[i].StartedAt.Before(sessions[j].StartedAt)
		}
		return sessions[i].Key < sessions[j].Key
	})
}

// Executions turns parsed sessions into ingest payloads. It is pure: identical
// sessions always produce identical payloads, so a re-run is recognised as a
// duplicate by the platform.
func Executions(agent Agent, sessions []Session) []domain.InsightExecution {
	executions := make([]domain.InsightExecution, 0, len(sessions))
	for _, session := range sessions {
		entries := mergedUsage(session.Usage)
		if len(entries) == 0 {
			continue
		}
		// A session with no usable timestamp cannot satisfy the platform's
		// validity window; drop it instead of sending a refused payload.
		if session.StartedAt.IsZero() && session.FinishedAt.IsZero() {
			continue
		}
		started := truncateMS(session.StartedAt)
		finished := truncateMS(session.FinishedAt)
		if started.IsZero() {
			started = finished
		}
		if finished.IsZero() {
			finished = started
		}
		if finished.Before(started) {
			finished = started
		}
		status := session.Status
		if status == "" {
			status = "succeeded"
		}
		id := executionID(agent, session.Key)
		executions = append(executions, domain.InsightExecution{
			ID:                  id,
			Type:                "workflow_execution",
			ExternalExecutionID: id,
			Workflow: domain.InsightWorkflow{
				ID:   string(agent),
				Name: agent.DisplayName(),
			},
			StartedAt:           started,
			FinishedAt:          finished,
			ExecutionDurationMs: session.ExecutionDurationMs,
			Status:              status,
			Usage:               entries,
		})
	}
	sort.Slice(executions, func(i, j int) bool {
		if !executions[i].StartedAt.Equal(executions[j].StartedAt) {
			return executions[i].StartedAt.Before(executions[j].StartedAt)
		}
		return executions[i].ID < executions[j].ID
	})
	return executions
}

// mergedUsage sorts usage by provider/model, merges entries that collide after
// name clipping, and caps the list at the platform's limit of 64 entries.
func mergedUsage(entries []domain.InsightUsage) []domain.InsightUsage {
	if len(entries) == 0 {
		return nil
	}
	sorted := append([]domain.InsightUsage(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Provider != sorted[j].Provider {
			return sorted[i].Provider < sorted[j].Provider
		}
		return sorted[i].Model < sorted[j].Model
	})
	merged := make([]domain.InsightUsage, 0, len(sorted))
	for _, entry := range sorted {
		if len(merged) > 0 && merged[len(merged)-1].Provider == entry.Provider && merged[len(merged)-1].Model == entry.Model {
			last := &merged[len(merged)-1]
			last.InputTokens += entry.InputTokens
			last.OutputTokens += entry.OutputTokens
			last.CacheReadTokens += entry.CacheReadTokens
			last.CacheWriteTokens += entry.CacheWriteTokens
			last.ReasoningTokens += entry.ReasoningTokens
			if last.CostUSD != nil && entry.CostUSD != nil && last.CostStatus != "unknown" && entry.CostStatus != "unknown" {
				last.CostUSD = new(*last.CostUSD + *entry.CostUSD)
				if last.CostStatus == "estimated" || entry.CostStatus == "estimated" {
					last.CostStatus = "estimated"
				}
			} else {
				last.CostUSD = nil
				last.CostStatus = "unknown"
			}
			continue
		}
		merged = append(merged, entry)
	}
	if len(merged) > 64 {
		// Keep the models that consumed the most input; the platform refuses
		// more than 64 entries per execution.
		sort.SliceStable(merged, func(i, j int) bool { return merged[i].InputTokens > merged[j].InputTokens })
		merged = merged[:64]
		sort.Slice(merged, func(i, j int) bool {
			if merged[i].Provider != merged[j].Provider {
				return merged[i].Provider < merged[j].Provider
			}
			return merged[i].Model < merged[j].Model
		})
	}
	return merged
}

// usage builds one platform usage entry. cost is nil when the source records
// no real cost; the platform then estimates from its price table.
func usage(provider, model string, input, output, cacheRead, cacheWrite, reasoning int64, cost *float64) domain.InsightUsage {
	status := "unknown"
	if cost != nil {
		status = "reported"
	}
	return usageWithStatus(provider, model, input, output, cacheRead, cacheWrite, reasoning, cost, status)
}

// usageWithStatus is usage with the cost status chosen by the adapter, so a
// partially priced session can report a lower-bound cost as "estimated".
func usageWithStatus(provider, model string, input, output, cacheRead, cacheWrite, reasoning int64, cost *float64, status string) domain.InsightUsage {
	entry := domain.InsightUsage{
		Provider:         clip(strings.TrimSpace(provider), 128),
		Model:            clip(strings.TrimSpace(model), 128),
		InputTokens:      input,
		OutputTokens:     output,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
		ReasoningTokens:  reasoning,
		CostStatus:       "unknown",
	}
	if entry.Provider == "" {
		entry.Provider = "unknown"
	}
	if entry.Model == "" {
		entry.Model = "unknown"
	}
	if entry.CacheReadTokens+entry.CacheWriteTokens > entry.InputTokens {
		entry.InputTokens = entry.CacheReadTokens + entry.CacheWriteTokens
	}
	if entry.ReasoningTokens > entry.OutputTokens {
		entry.ReasoningTokens = 0
	}
	if status != "unknown" && cost != nil {
		entry.CostUSD = cost
		entry.CostStatus = status
	}
	return entry
}

// inclusiveInput applies the platform rule that cache reads and writes are
// subsets of input. Sources that store the non-cached prompt tokens separately
// must add the cache counts before reporting.
func inclusiveInput(nonCached, cacheRead, cacheWrite int64) int64 {
	return nonCached + cacheRead + cacheWrite
}

func usageKey(provider, model string) string {
	return provider + "\x00" + model
}

// executionID bounds the identifier to the platform's 256-byte limit while
// staying deterministic when a path is unusually long.
func executionID(agent Agent, key string) string {
	id := string(agent) + ":" + key
	if len(id) <= 256 {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return string(agent) + ":sha256:" + hex.EncodeToString(sum[:])
}

func truncateMS(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Truncate(time.Millisecond)
}

func clip(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	// Clipping mid-rune would produce invalid UTF-8 in the payload; drop the
	// partial rune.
	cut := limit
	for cut > 0 && value[cut]&0xC0 == 0x80 {
		cut--
	}
	return value[:cut]
}

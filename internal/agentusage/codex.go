package agentusage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"agentflow/internal/domain"
)

// CodexReader reads Codex rollout files (~/.codex/sessions/**/rollout-*.jsonl).
// One rollout file is one session; the last total_token_usage record carries
// the session totals.
type CodexReader struct {
	root string
}

// NewCodex builds a reader rooted at the Codex data directory (usually ~/.codex).
func NewCodex(root string) *CodexReader {
	if root == "" {
		root = filepath.Join(homeDir(), ".codex")
	}
	return &CodexReader{root: preferChild(root, "sessions")}
}

func (r *CodexReader) Agent() Agent { return AgentCodex }
func (r *CodexReader) Root() string { return r.root }

type codexUsage struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
	Reasoning  int64 `json:"reasoning_output_tokens"`
}

type codexRecord struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type          string `json:"type"`
		Timestamp     string `json:"timestamp"`
		ModelProvider string `json:"model_provider"`
		Model         string `json:"model"`
		Info          *struct {
			Total *codexUsage `json:"total_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

func (r *CodexReader) Read(ctx context.Context) ([]Session, error) {
	paths, err := walkJSONL(r.root, func(name string) bool {
		return strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, ".jsonl")
	})
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		session, ok, err := readCodexRollout(path)
		if err != nil {
			return nil, err
		}
		if ok {
			sessions = append(sessions, session)
		}
	}
	sortSessions(sessions)
	return sessions, nil
}

func readCodexRollout(path string) (Session, bool, error) {
	var (
		times    timeRange
		provider string
		model    string
		total    *codexUsage
	)
	err := scanLines(path, func(line []byte) error {
		var record codexRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil // tolerant: a damaged line does not hide the rest
		}
		if ts, ok := parseTimestamp(record.Timestamp); ok {
			times.add(ts)
		}
		if ts, ok := parseTimestamp(record.Payload.Timestamp); ok {
			times.add(ts)
		}
		if record.Payload.ModelProvider != "" {
			provider = record.Payload.ModelProvider
		}
		if record.Payload.Model != "" {
			model = record.Payload.Model
		}
		if record.Payload.Info != nil && record.Payload.Info.Total != nil {
			total = record.Payload.Info.Total
		}
		return nil
	})
	if err != nil {
		return Session{}, false, err
	}
	if total == nil {
		return Session{}, false, nil
	}
	started, finished := times.settle(path)
	return Session{
		Key:        sessionKeyFromPath(path),
		StartedAt:  started,
		FinishedAt: finished,
		Usage: []domain.InsightUsage{
			usage(provider, model,
				total.Input, total.Output,
				total.Cached, total.CacheWrite, total.Reasoning, nil),
		},
	}, true, nil
}

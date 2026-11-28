package agentusage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
)

// ClaudeReader reads Claude Code transcripts (~/.claude/projects/**/*.jsonl).
// The format is an internal implementation detail of Claude Code, so parsing
// is tolerant: records without usage are skipped and damaged lines are ignored.
type ClaudeReader struct {
	root string
}

// NewClaude builds a reader rooted at the Claude Code data directory
// (usually ~/.claude).
func NewClaude(root string) *ClaudeReader {
	if root == "" {
		root = filepath.Join(homeDir(), ".claude")
	}
	return &ClaudeReader{root: preferChild(root, "projects")}
}

func (r *ClaudeReader) Agent() Agent { return AgentClaude }
func (r *ClaudeReader) Root() string { return r.root }

type claudeRecord struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   *struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens         int64 `json:"input_tokens"`
			OutputTokens        int64 `json:"output_tokens"`
			CacheReadTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
			ReasoningTokens     int64 `json:"reasoning_output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type claudeGroup struct {
	nonCachedInput int64
	output         int64
	cacheRead      int64
	cacheWrite     int64
	reasoning      int64
}

func (r *ClaudeReader) Read(ctx context.Context) ([]Session, error) {
	paths, err := walkJSONL(r.root, func(name string) bool { return strings.HasSuffix(name, ".jsonl") })
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		session, ok, err := readClaudeSession(path)
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

func readClaudeSession(path string) (Session, bool, error) {
	var (
		times  timeRange
		groups = map[string]*claudeGroup{}
	)
	err := scanLines(path, func(line []byte) error {
		var record claudeRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil // tolerant: Claude Code evolves this format
		}
		if ts, ok := parseTimestamp(record.Timestamp); ok {
			times.add(ts)
		}
		if record.Message == nil || record.Message.Usage == nil {
			return nil
		}
		used := record.Message.Usage
		if used.InputTokens == 0 && used.OutputTokens == 0 && used.CacheReadTokens == 0 && used.CacheCreationTokens == 0 {
			return nil
		}
		model := record.Message.Model
		if model == "" {
			model = "unknown"
		}
		group, ok := groups[model]
		if !ok {
			group = &claudeGroup{}
			groups[model] = group
		}
		group.nonCachedInput += used.InputTokens
		group.output += used.OutputTokens
		group.cacheRead += used.CacheReadTokens
		group.cacheWrite += used.CacheCreationTokens
		group.reasoning += used.ReasoningTokens
		return nil
	})
	if err != nil {
		return Session{}, false, err
	}
	if len(groups) == 0 {
		return Session{}, false, nil
	}
	started, finished := times.settle(path)
	session := Session{
		Key:        sessionKeyFromPath(path),
		StartedAt:  started,
		FinishedAt: finished,
	}
	for model, group := range groups {
		session.Usage = append(session.Usage, usage(
			"anthropic", model,
			inclusiveInput(group.nonCachedInput, group.cacheRead, group.cacheWrite),
			group.output, group.cacheRead, group.cacheWrite, group.reasoning, nil,
		))
	}
	return session, true, nil
}

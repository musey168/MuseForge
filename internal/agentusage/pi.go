package agentusage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
)

// PiReader reads pi session files (~/.pi/agent/sessions/**/*.jsonl).
// A session file is one session; assistant messages carry the usage.
type PiReader struct {
	root string
}

// NewPi builds a reader rooted at the pi data directory (usually ~/.pi).
func NewPi(root string) *PiReader {
	if root == "" {
		root = filepath.Join(homeDir(), ".pi")
	}
	return &PiReader{root: preferChild(root, filepath.Join("agent", "sessions"))}
}

func (r *PiReader) Agent() Agent { return AgentPi }
func (r *PiReader) Root() string { return r.root }

type piRecord struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Provider  string `json:"provider"`
	ModelID   string `json:"modelId"`
	Message   *struct {
		Provider string   `json:"provider"`
		Model    string   `json:"model"`
		Usage    *piUsage `json:"usage"`
	} `json:"message"`
}

type piUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Cost       *struct {
		Total *float64 `json:"total"`
	} `json:"cost"`
}

// piGroup accumulates one provider/model's usage inside a session.
type piGroup struct {
	provider       string
	model          string
	nonCachedInput int64
	output         int64
	cacheRead      int64
	cacheWrite     int64
	cost           float64
	costKnown      bool
}

func (r *PiReader) Read(ctx context.Context) ([]Session, error) {
	paths, err := walkJSONL(r.root, func(name string) bool { return strings.HasSuffix(name, ".jsonl") })
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		session, ok, err := readPiSession(path)
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

func readPiSession(path string) (Session, bool, error) {
	var (
		times       timeRange
		lastProfile struct{ provider, model string }
		groups      = map[string]*piGroup{}
	)
	err := scanLines(path, func(line []byte) error {
		var record piRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil // tolerant: an unknown or damaged record is ignored
		}
		if ts, ok := parseTimestamp(record.Timestamp); ok {
			times.add(ts)
		}
		if record.Provider != "" && record.ModelID != "" {
			lastProfile.provider, lastProfile.model = record.Provider, record.ModelID
		}
		if record.Message == nil {
			return nil
		}
		if record.Message.Provider != "" && record.Message.Model != "" {
			lastProfile.provider, lastProfile.model = record.Message.Provider, record.Message.Model
		}
		if record.Message.Usage == nil {
			return nil
		}
		provider, model := record.Message.Provider, record.Message.Model
		if provider == "" {
			provider = lastProfile.provider
		}
		if model == "" {
			model = lastProfile.model
		}
		key := usageKey(provider, model)
		group, ok := groups[key]
		if !ok {
			group = &piGroup{provider: provider, model: model, costKnown: true}
			groups[key] = group
		}
		used := record.Message.Usage
		group.nonCachedInput += used.Input
		group.output += used.Output
		group.cacheRead += used.CacheRead
		group.cacheWrite += used.CacheWrite
		if used.Cost != nil && used.Cost.Total != nil {
			group.cost += *used.Cost.Total
		} else {
			group.costKnown = false
		}
		return nil
	})
	if err != nil {
		return Session{}, false, err
	}
	if len(groups) == 0 {
		return Session{}, false, nil
	}
	started, finished := times.settle(path)
	session := Session{Key: sessionKeyFromPath(path), StartedAt: started, FinishedAt: finished}
	for _, group := range groups {
		var cost *float64
		if group.costKnown {
			cost = new(group.cost)
		}
		session.Usage = append(session.Usage, usage(
			group.provider, group.model,
			inclusiveInput(group.nonCachedInput, group.cacheRead, group.cacheWrite),
			group.output, group.cacheRead, group.cacheWrite, 0, cost,
		))
	}
	return session, true, nil
}

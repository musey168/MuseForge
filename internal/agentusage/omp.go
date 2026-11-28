package agentusage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

// OMPReader reads omp's message accounting from ~/.omp/stats.db. omp stores
// non-cached prompt tokens separately from cache reads and writes.
type OMPReader struct {
	root string
}

// NewOMP builds a reader rooted at the omp data directory (usually ~/.omp).
func NewOMP(root string) *OMPReader {
	if root == "" {
		root = filepath.Join(homeDir(), ".omp")
	}
	return &OMPReader{root: root}
}

func (r *OMPReader) Agent() Agent { return AgentOMP }
func (r *OMPReader) Root() string { return filepath.Join(r.root, "stats.db") }

const ompQuery = `
SELECT session_file, provider, model,
       MIN(timestamp), MAX(timestamp),
       SUM(input_tokens), SUM(output_tokens),
       SUM(cache_read_tokens), SUM(cache_write_tokens),
       SUM(cost_total), SUM(cost_unpriced),
       SUM(COALESCE(duration, 0)),
       SUM(CASE WHEN stop_reason = 'error' OR error_message IS NOT NULL THEN 1 ELSE 0 END),
       SUM(CASE WHEN stop_reason = 'aborted' THEN 1 ELSE 0 END)
FROM messages
GROUP BY session_file, provider, model
ORDER BY session_file, provider, model`

func (r *OMPReader) Read(ctx context.Context) ([]Session, error) {
	dbPath := filepath.Join(r.root, "stats.db")
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open omp stats database: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, ompQuery)
	if err != nil {
		return nil, fmt.Errorf("read omp stats database: %w", err)
	}
	defer rows.Close()

	sessions := map[string]*Session{}
	durations := map[string]float64{}
	failed := map[string]int64{}
	aborted := map[string]int64{}
	for rows.Next() {
		var (
			sessionFile, provider, model string
			firstMs, lastMs              int64
			input, output                int64
			cacheRead, cacheWrite        int64
			cost                         float64
			unpriced                     int64
			duration                     float64
			failedRows, abortedRows      int64
		)
		if err := rows.Scan(&sessionFile, &provider, &model, &firstMs, &lastMs,
			&input, &output, &cacheRead, &cacheWrite, &cost, &unpriced,
			&duration, &failedRows, &abortedRows); err != nil {
			return nil, fmt.Errorf("read omp stats row: %w", err)
		}
		session, ok := sessions[sessionFile]
		if !ok {
			session = &Session{Key: sessionKeyFromPath(sessionFile)}
			sessions[sessionFile] = session
		}
		started := time.UnixMilli(firstMs).UTC()
		finished := time.UnixMilli(lastMs).UTC()
		session.addSpan(started, finished)
		durations[sessionFile] += duration
		failed[sessionFile] += failedRows
		aborted[sessionFile] += abortedRows

		// Mirror the live omp extension: an unpriced group with a positive
		// total is a lower bound ("estimated"); a zero total is unknown.
		status := "reported"
		var costPtr *float64
		switch {
		case unpriced == 0:
			costPtr = new(cost)
		case cost > 0:
			status = "estimated"
			costPtr = new(cost)
		default:
			status = "unknown"
		}
		session.Usage = append(session.Usage, usageWithStatus(
			provider, model,
			inclusiveInput(input, cacheRead, cacheWrite), output,
			cacheRead, cacheWrite, 0, costPtr, status,
		))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read omp stats rows: %w", err)
	}
	out := make([]Session, 0, len(sessions))
	for file, session := range sessions {
		if failed[file] > 0 {
			session.Status = "failed"
		} else if aborted[file] > 0 {
			session.Status = "cancelled"
		}
		session.ExecutionDurationMs = new(int64(durations[file] + 0.5))
		out = append(out, *session)
	}
	sortSessions(out)
	return out, nil
}

// addSpan widens the session's wall-clock span to include one group's range.
func (s *Session) addSpan(first, last time.Time) {
	if s.StartedAt.IsZero() || first.Before(s.StartedAt) {
		s.StartedAt = first
	}
	if s.FinishedAt.IsZero() || last.After(s.FinishedAt) {
		s.FinishedAt = last
	}
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

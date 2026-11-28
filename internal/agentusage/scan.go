package agentusage

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// scanLines calls fn with every non-empty line of path. Read errors are
// returned; malformed content is the caller's business because the formats
// differ in what "malformed" means.
func scanLines(path string, fn func(line []byte) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 1<<20)
	for {
		line, readErr := reader.ReadBytes('\n')
		if trimmed := trimLine(line); len(trimmed) > 0 {
			if err := fn(trimmed); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func trimLine(line []byte) []byte {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return nil
	}
	return []byte(trimmed)
}

// walkJSONL lists the files under root whose names match match, sorted so a
// run is deterministic. A missing root is not an error: the agent is simply
// not installed on this machine.
func walkJSONL(root string, match func(name string) bool) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !match(entry.Name()) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// sessionKeyFromPath is the stable session identity shared by every sender:
// the file's basename without its .jsonl suffix, bounded to 200 bytes (the
// platform caps execution IDs at 256, and the agent prefix must fit).
func sessionKeyFromPath(path string) string {
	name := filepath.Base(path)
	if len(name) >= 6 && strings.EqualFold(name[len(name)-6:], ".jsonl") {
		name = name[:len(name)-6]
	}
	return clip(name, 200)
}

// fileTime is the fallback when a record carries no usable timestamp.
func fileTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime().UTC()
}

func parseTimestamp(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

// timeRange tracks the earliest and latest timestamp seen in a file.
type timeRange struct {
	first time.Time
	last  time.Time
}

func (r *timeRange) add(t time.Time) {
	if t.IsZero() {
		return
	}
	if r.first.IsZero() || t.Before(r.first) {
		r.first = t
	}
	if r.last.IsZero() || t.After(r.last) {
		r.last = t
	}
}

func (r *timeRange) settle(path string) (time.Time, time.Time) {
	if r.first.IsZero() || r.last.IsZero() {
		fallback := fileTime(path)
		return fallback, fallback
	}
	return r.first, r.last
}

// preferChild resolves an optional nested data directory so that both the
// agent's data root and the nested directory itself work as --root.
func preferChild(root, child string) string {
	candidate := filepath.Join(root, child)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return root
}

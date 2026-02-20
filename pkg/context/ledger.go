// Package context provides the longitudinal context ledger for Burrow.
// Entries are stored as markdown files with YAML front matter, organized
// by type subdirectory. The ledger never leaves the local machine.
package context

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jcadam/burrow/pkg/slug"
)

// Entry types for the context ledger.
const (
	TypeReport  = "report"
	TypeResult  = "result"
	TypeSession = "session"
	TypeContact = "contact"
)

// Entry is a single item in the context ledger.
type Entry struct {
	ID        string
	Type      string // report | result | session | contact
	Label     string
	Routine   string
	Timestamp time.Time
	Content   string
}

// Ledger manages the context ledger stored on disk.
type Ledger struct {
	root string
	mu   sync.Mutex
}

// NewLedger creates a ledger rooted at the given directory.
func NewLedger(root string) (*Ledger, error) {
	for _, sub := range []string{TypeReport + "s", TypeResult + "s", TypeSession + "s", TypeContact + "s"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return nil, fmt.Errorf("creating context directory %s: %w", sub, err)
		}
	}
	return &Ledger{root: root}, nil
}

// Append writes an entry to disk as a markdown file with YAML front matter.
// If a file with the same timestamp and slug already exists, an incrementing
// index (-2, -3, etc.) is appended to avoid collisions.
func (l *Ledger) Append(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := e.Timestamp.Format("2006-01-02T150405")
	nameSlug := slug.Sanitize(e.Label)
	dir := filepath.Join(l.root, e.Type+"s")

	// Find a unique filename
	filename := fmt.Sprintf("%s-%s.md", ts, nameSlug)
	path := filepath.Join(dir, filename)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		filename = fmt.Sprintf("%s-%s-%d.md", ts, nameSlug, i)
		path = filepath.Join(dir, filename)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("type: %s\n", e.Type))
	b.WriteString(fmt.Sprintf("label: %q\n", e.Label))
	if e.Routine != "" {
		b.WriteString(fmt.Sprintf("routine: %s\n", e.Routine))
	}
	b.WriteString(fmt.Sprintf("timestamp: %s\n", e.Timestamp.Format(time.RFC3339)))
	b.WriteString("---\n\n")
	b.WriteString(e.Content)

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Search returns entries matching query (case-insensitive substring) across all types.
// Results are sorted newest first.
func (l *Ledger) Search(query string) ([]Entry, error) {
	query = strings.ToLower(query)
	var entries []Entry

	for _, sub := range []string{TypeReport + "s", TypeResult + "s", TypeSession + "s", TypeContact + "s"} {
		dir := filepath.Join(l.root, sub)
		files, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				continue
			}
			content := string(data)
			if strings.Contains(strings.ToLower(content), query) {
				entry := parseEntry(content, f.Name(), sub)
				entries = append(entries, entry)
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	return entries, nil
}

// List returns entries of a given type, newest first, up to limit.
// If limit <= 0, all entries are returned.
func (l *Ledger) List(entryType string, limit int) ([]Entry, error) {
	dir := filepath.Join(l.root, entryType+"s")
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []Entry
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		entries = append(entries, parseEntry(string(data), f.Name(), entryType+"s"))
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	return entries, nil
}

// GatherContext concatenates recent entries up to maxBytes for LLM context.
func (l *Ledger) GatherContext(maxBytes int) (string, error) {
	var all []Entry

	for _, sub := range []string{TypeReport + "s", TypeResult + "s", TypeSession + "s", TypeContact + "s"} {
		dir := filepath.Join(l.root, sub)
		files, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				continue
			}
			all = append(all, parseEntry(string(data), f.Name(), sub))
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})

	var b strings.Builder
	for _, e := range all {
		chunk := fmt.Sprintf("## %s (%s)\n%s\n\n", e.Label, e.Timestamp.Format("2006-01-02 15:04"), e.Content)
		if b.Len()+len(chunk) > maxBytes {
			break
		}
		b.WriteString(chunk)
	}

	return b.String(), nil
}

// TypeStats holds aggregate statistics for one entry type.
type TypeStats struct {
	Count    int
	Bytes    int64
	Earliest time.Time
	Latest   time.Time
}

// Stats returns aggregate statistics for each entry type in the ledger.
func (l *Ledger) Stats() (map[string]TypeStats, error) {
	stats := make(map[string]TypeStats)

	for _, sub := range []string{TypeReport + "s", TypeResult + "s", TypeSession + "s", TypeContact + "s"} {
		entryType := strings.TrimSuffix(sub, "s")
		dir := filepath.Join(l.root, sub)
		files, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		var ts TypeStats
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			ts.Count++
			ts.Bytes += info.Size()

			// Read only the front matter (first ~512 bytes) to extract timestamp.
			header, err := readHead(filepath.Join(dir, f.Name()), 512)
			if err != nil {
				continue
			}
			entry := parseEntry(header, f.Name(), sub)
			if !entry.Timestamp.IsZero() {
				if ts.Earliest.IsZero() || entry.Timestamp.Before(ts.Earliest) {
					ts.Earliest = entry.Timestamp
				}
				if ts.Latest.IsZero() || entry.Timestamp.After(ts.Latest) {
					ts.Latest = entry.Timestamp
				}
			}
		}

		if ts.Count > 0 {
			stats[entryType] = ts
		}
	}

	return stats, nil
}

// readHead reads up to n bytes from the beginning of a file.
func readHead(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, n)
	read, err := f.Read(buf)
	if err != nil && read == 0 {
		return "", err
	}
	return string(buf[:read]), nil
}

// parseEntry extracts an Entry from raw file content and filename.
func parseEntry(raw, filename, subdir string) Entry {
	e := Entry{
		ID: filename,
	}

	// Derive type from subdir
	e.Type = strings.TrimSuffix(subdir, "s")

	// Parse YAML front matter
	if strings.HasPrefix(raw, "---\n") {
		end := strings.Index(raw[4:], "---\n")
		if end >= 0 {
			front := raw[4 : 4+end]
			e.Content = strings.TrimSpace(raw[4+end+4:])

			for _, line := range strings.Split(front, "\n") {
				key, val, ok := strings.Cut(line, ": ")
				if !ok {
					continue
				}
				val = strings.TrimSpace(val)
				val = strings.Trim(val, `"`)
				switch key {
				case "label":
					e.Label = val
				case "routine":
					e.Routine = val
				case "timestamp":
					if t, err := time.Parse(time.RFC3339, val); err == nil {
						e.Timestamp = t
					}
				case "type":
					e.Type = val
				}
			}
		} else {
			// No closing --- found; treat the entire file as content
			e.Content = strings.TrimSpace(raw[4:])
		}
	}

	return e
}


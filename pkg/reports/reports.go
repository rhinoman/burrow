// Package reports handles report storage, retrieval, and listing.
package reports

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Report represents a generated report on disk.
type Report struct {
	Dir      string   // directory path
	Routine  string   // routine name that generated it
	Title    string   // report title
	Date     string   // YYYY-MM-DD
	Markdown string   // report content
	Sources  []string // list of source files in data/
}

// Save writes a report to disk under baseDir/YYYY-MM-DDT150405-routine-name/.
// The second-precision timestamp prevents collisions when running the same routine
// multiple times per day. Raw results are stored in a data/ subdirectory.
func Save(baseDir string, routine string, markdown string, rawResults map[string][]byte) (*Report, error) {
	now := time.Now()
	date := now.Format("2006-01-02")
	dirName := now.Format("2006-01-02T150405") + "-" + sanitize(routine)
	reportDir := filepath.Join(baseDir, dirName)

	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating report directory: %w", err)
	}

	reportPath := filepath.Join(reportDir, "report.md")
	if err := os.WriteFile(reportPath, []byte(markdown), 0o644); err != nil {
		return nil, fmt.Errorf("writing report: %w", err)
	}

	var sources []string
	if len(rawResults) > 0 {
		dataDir := filepath.Join(reportDir, "data")
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating data directory: %w", err)
		}
		for name, data := range rawResults {
			path := filepath.Join(dataDir, sanitize(name)+".json")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return nil, fmt.Errorf("writing raw result %q: %w", name, err)
			}
			sources = append(sources, path)
		}
	}

	return &Report{
		Dir:      reportDir,
		Routine:  routine,
		Date:     date,
		Markdown: markdown,
		Sources:  sources,
	}, nil
}

// Load reads a report from a directory.
func Load(reportDir string) (*Report, error) {
	reportPath := filepath.Join(reportDir, "report.md")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("reading report: %w", err)
	}

	base := filepath.Base(reportDir)
	date, routine := parseReportDirName(base)

	var sources []string
	dataDir := filepath.Join(reportDir, "data")
	if entries, err := os.ReadDir(dataDir); err == nil {
		for _, e := range entries {
			sources = append(sources, filepath.Join(dataDir, e.Name()))
		}
	}

	// Extract title from first markdown heading
	title := extractTitle(string(data))

	return &Report{
		Dir:      reportDir,
		Routine:  routine,
		Title:    title,
		Date:     date,
		Markdown: string(data),
		Sources:  sources,
	}, nil
}

// List returns all reports in the base directory, sorted newest first.
func List(baseDir string) ([]*Report, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing reports: %w", err)
	}

	var reports []*Report
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		reportPath := filepath.Join(baseDir, e.Name(), "report.md")
		if _, err := os.Stat(reportPath); os.IsNotExist(err) {
			continue
		}
		r, err := Load(filepath.Join(baseDir, e.Name()))
		if err != nil {
			continue // skip unreadable reports
		}
		reports = append(reports, r)
	}

	// Sort by directory basename (includes THHMM timestamp) for minute-level ordering
	sort.Slice(reports, func(i, j int) bool {
		return filepath.Base(reports[i].Dir) > filepath.Base(reports[j].Dir)
	})

	return reports, nil
}

// FindLatest returns the most recent report for a given routine, or nil if none.
// Scans directory names directly rather than loading every report from disk.
func FindLatest(baseDir string, routine string) (*Report, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing reports: %w", err)
	}

	sanitized := sanitize(routine)
	var candidates []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		_, name := parseReportDirName(e.Name())
		if name == sanitized {
			candidates = append(candidates, e.Name())
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Directory names sort lexicographically by date; take the last one
	sort.Strings(candidates)
	return Load(filepath.Join(baseDir, candidates[len(candidates)-1]))
}

func sanitize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	// Remove anything that's not alphanumeric, dash, or underscore
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// datePattern matches YYYY-MM-DD, YYYY-MM-DDTHHMM, or YYYY-MM-DDTHHMMSS at the start of a directory name.
var datePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})(T\d{4,6})?-(.+)$`)

func parseReportDirName(name string) (date, routine string) {
	m := datePattern.FindStringSubmatch(name)
	if m == nil {
		return "", name
	}
	return m[1], m[3]
}

func extractTitle(markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

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

	"github.com/jcadam/burrow/pkg/slug"
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

// Create writes raw results to disk under baseDir/YYYY-MM-DDT150405-routine-name/data/.
// It returns the report directory path. Call Finish after synthesis to write report.md.
// This ensures raw results are persisted before synthesis (spec §4.1).
func Create(baseDir string, routine string, rawResults map[string][]byte) (string, error) {
	now := time.Now()
	dirName := now.Format("2006-01-02T150405") + "-" + slug.Sanitize(routine)
	reportDir := filepath.Join(baseDir, dirName)

	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return "", fmt.Errorf("creating report directory: %w", err)
	}

	if len(rawResults) > 0 {
		dataDir := filepath.Join(reportDir, "data")
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return "", fmt.Errorf("creating data directory: %w", err)
		}
		for name, data := range rawResults {
			// Raw results are stored as .json — REST services return JSON overwhelmingly.
			// If non-JSON sources are added, detect content type here.
			path := filepath.Join(dataDir, slug.Sanitize(name)+".json")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return "", fmt.Errorf("writing raw result %q: %w", name, err)
			}
		}
	}

	return reportDir, nil
}

// Finish writes the synthesized markdown to an existing report directory
// and returns the completed Report.
func Finish(reportDir string, routine string, markdown string) (*Report, error) {
	reportPath := filepath.Join(reportDir, "report.md")
	if err := os.WriteFile(reportPath, []byte(markdown), 0o644); err != nil {
		return nil, fmt.Errorf("writing report: %w", err)
	}

	date, _ := parseReportDirName(filepath.Base(reportDir))

	var sources []string
	dataDir := filepath.Join(reportDir, "data")
	if entries, err := os.ReadDir(dataDir); err == nil {
		for _, e := range entries {
			sources = append(sources, filepath.Join(dataDir, e.Name()))
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

// Save is a convenience wrapper that calls Create then Finish in sequence.
func Save(baseDir string, routine string, markdown string, rawResults map[string][]byte) (*Report, error) {
	reportDir, err := Create(baseDir, routine, rawResults)
	if err != nil {
		return nil, err
	}
	return Finish(reportDir, routine, markdown)
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

	// Sort by directory basename (includes THHMMSS timestamp) for second-level ordering
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

	sanitized := slug.Sanitize(routine)
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

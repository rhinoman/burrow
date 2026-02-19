// Package slug provides filename-safe string sanitization for Burrow.
package slug

import "strings"

// Sanitize converts a string into a safe filename component.
// Lowercases, replaces non-alphanumeric characters with dashes,
// collapses runs of dashes, and trims leading/trailing dashes.
func Sanitize(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		return "unknown"
	}
	return s
}

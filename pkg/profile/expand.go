package profile

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"
)

// legacySyntax matches {{profile.field_name}} and {{ profile.field_name }}
// (with optional spaces) for backward-compatible conversion to Go template
// function call syntax: {{profile "field_name"}}.
var legacySyntax = regexp.MustCompile(`\{\{\s*profile\.([a-zA-Z_][a-zA-Z0-9_.]*)\s*\}\}`)

// convertLegacySyntax rewrites old {{profile.X}} references to {{profile "X"}}
// before Go template parsing. Preserves surrounding whitespace within delimiters.
func convertLegacySyntax(text string) string {
	return legacySyntax.ReplaceAllStringFunc(text, func(match string) string {
		subs := legacySyntax.FindStringSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		return `{{profile "` + subs[1] + `"}}`
	})
}

// templateContext tracks unresolved profile references during template execution.
type templateContext struct {
	profile    *Profile
	unresolved []string
}

// profileFunc is the template function for {{profile "key"}}. Returns the
// original {{profile.key}} text for missing keys (spec: unresolved references
// are left as-is).
func (tc *templateContext) profileFunc(key string) string {
	val, ok := tc.profile.Get(key)
	if !ok {
		tc.unresolved = append(tc.unresolved, key)
		return "{{profile." + key + "}}"
	}
	return val
}

// buildFuncMap returns the template.FuncMap with all built-in functions.
func buildFuncMap(tc *templateContext) template.FuncMap {
	now := time.Now()
	return template.FuncMap{
		"profile":   tc.profileFunc,
		"today":     func() string { return now.Format("2006-01-02") },
		"yesterday": func() string { return now.AddDate(0, 0, -1).Format("2006-01-02") },
		"now":       func() string { return now.Format(time.RFC3339) },
		"year":      func() string { return now.Format("2006") },
		"month":     func() string { return now.Format("01") },
		"day":       func() string { return now.Format("02") },
		"date": func(layout, dateStr string) string {
			for _, parseLayout := range []string{"2006-01-02", time.RFC3339, "01/02/2006"} {
				if t, err := time.Parse(parseLayout, dateStr); err == nil {
					return t.Format(layout)
				}
			}
			return dateStr // unparseable — pass through
		},
		"split": func(s, sep string) []string { return strings.Split(s, sep) },
		"join":  func(sep string, s []string) string { return strings.Join(s, sep) },
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
	}
}

// Expand replaces template references in text with values from the profile
// and built-in functions. Supports Go text/template syntax with a backward-
// compatible shim for old {{profile.X}} syntax.
//
// Nil-safe: returns text unchanged when profile is nil.
//
// Unresolved references are left as-is so the user sees what's missing.
// The returned error lists unresolved fields (execution should continue).
func Expand(text string, p *Profile) (string, error) {
	if p == nil || text == "" {
		return text, nil
	}

	// Convert legacy syntax before Go template parsing.
	converted := convertLegacySyntax(text)

	tc := &templateContext{profile: p}
	fm := buildFuncMap(tc)

	tmpl, err := template.New("expand").Funcs(fm).Parse(converted)
	if err != nil {
		// Go template parse error — fall back to legacy regex expander.
		return legacyExpand(text, p)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		// Execution error — fall back to legacy regex expander.
		return legacyExpand(text, p)
	}

	result := buf.String()
	if len(tc.unresolved) > 0 {
		return result, fmt.Errorf("unresolved profile fields: %s", strings.Join(tc.unresolved, ", "))
	}
	return result, nil
}

// legacyProfilePattern matches {{profile.field_name}} references (no spaces).
// Supports dot-notation for nested keys (e.g., {{profile.location.latitude}}).
var legacyProfilePattern = regexp.MustCompile(`\{\{profile\.([a-zA-Z_][a-zA-Z0-9_.]*)\}\}`)

// legacyExpand is the original regex-based expander, used as a fallback when
// Go template parsing fails (e.g., unbalanced delimiters in non-template text).
func legacyExpand(text string, p *Profile) (string, error) {
	if p == nil || text == "" {
		return text, nil
	}

	var unresolved []string

	result := legacyProfilePattern.ReplaceAllStringFunc(text, func(match string) string {
		subs := legacyProfilePattern.FindStringSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		key := subs[1]
		val, ok := p.Get(key)
		if !ok {
			unresolved = append(unresolved, key)
			return match // leave as-is
		}
		return val
	})

	if len(unresolved) > 0 {
		return result, fmt.Errorf("unresolved profile fields: %s", strings.Join(unresolved, ", "))
	}
	return result, nil
}

// ExpandParams expands template references in a params map.
// Returns a new map — the original is not modified (goroutine safety).
// Nil-safe: returns the original map unchanged when profile is nil.
func ExpandParams(params map[string]string, p *Profile) (map[string]string, error) {
	if p == nil || len(params) == 0 {
		return params, nil
	}

	expanded := make(map[string]string, len(params))
	var allUnresolved []string

	for k, v := range params {
		val, err := Expand(v, p)
		expanded[k] = val
		if err != nil {
			allUnresolved = append(allUnresolved, err.Error())
		}
	}

	if len(allUnresolved) > 0 {
		return expanded, fmt.Errorf("%s", strings.Join(allUnresolved, "; "))
	}
	return expanded, nil
}

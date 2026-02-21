package profile

import (
	"fmt"
	"regexp"
	"strings"
)

// profilePattern matches {{profile.field_name}} references.
var profilePattern = regexp.MustCompile(`\{\{profile\.([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

// Expand replaces {{profile.X}} references in text with values from the
// profile. Nil-safe: returns text unchanged when profile is nil.
//
// Unresolved references are left as-is so the user sees what's missing.
// The returned error lists unresolved fields (execution should continue).
func Expand(text string, p *Profile) (string, error) {
	if p == nil || text == "" {
		return text, nil
	}

	var unresolved []string

	result := profilePattern.ReplaceAllStringFunc(text, func(match string) string {
		subs := profilePattern.FindStringSubmatch(match)
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

// ExpandParams expands {{profile.X}} references in a params map.
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

// Package actions provides action parsing, clipboard access, system app handoff,
// and draft generation for Burrow reports.
package actions

import (
	"strings"
)

// ActionType identifies the kind of action suggested in a report.
type ActionType string

const (
	ActionDraft     ActionType = "draft"
	ActionOpen      ActionType = "open"
	ActionConfigure ActionType = "configure"
)

// Action represents a suggested action parsed from a report.
type Action struct {
	Type        ActionType
	Description string
	Target      string // URL, file path, or draft instruction
}

// ParseActions scans markdown text for action markers and returns the actions found.
// Recognized markers: [Draft], [Open], [Configure] — case-insensitive.
func ParseActions(markdown string) []Action {
	var actions []Action
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		var actionType ActionType
		var marker string

		switch {
		case strings.Contains(lower, "[draft]"):
			actionType = ActionDraft
			marker = "[draft]"
		case strings.Contains(lower, "[open]"):
			actionType = ActionOpen
			marker = "[open]"
		case strings.Contains(lower, "[configure]"):
			actionType = ActionConfigure
			marker = "[configure]"
		default:
			continue
		}

		// Extract description: everything after the marker
		idx := strings.Index(lower, marker)
		desc := strings.TrimSpace(trimmed[idx+len(marker):])
		// Extract target from description: look for parenthesized URL or path
		target := ""
		if pStart := strings.Index(desc, "("); pStart >= 0 {
			if pEnd := strings.Index(desc[pStart:], ")"); pEnd >= 0 {
				target = desc[pStart+1 : pStart+pEnd]
				desc = strings.TrimSpace(desc[:pStart] + desc[pStart+pEnd+1:])
			}
		}

		actions = append(actions, Action{
			Type:        actionType,
			Description: desc,
			Target:      target,
		})
	}
	return actions
}

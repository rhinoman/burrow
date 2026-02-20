package actions

import (
	"context"
	"strings"

	"github.com/jcadam/burrow/pkg/synthesis"
)

// Draft represents a generated communication draft.
type Draft struct {
	To      string
	Subject string
	Body    string
	Raw     string // Full LLM output
}

const draftSystemPrompt = `You are a professional communication drafting assistant.
Generate a draft based on the user's instruction and context data.
Format your response as:
To: [recipient]
Subject: [subject line]

[body text]

Keep the tone professional but natural. Be concise.`

// GenerateDraft uses an LLM to generate a communication draft.
func GenerateDraft(ctx context.Context, provider synthesis.Provider, instruction string, contextData string) (*Draft, error) {
	var userPrompt strings.Builder
	userPrompt.WriteString("Instruction: ")
	userPrompt.WriteString(instruction)
	if contextData != "" {
		userPrompt.WriteString("\n\nContext data:\n")
		userPrompt.WriteString(contextData)
	}

	raw, err := provider.Complete(ctx, draftSystemPrompt, userPrompt.String())
	if err != nil {
		return nil, err
	}

	return parseDraft(raw), nil
}

// parseDraft extracts structured fields from LLM output.
// Looks for "To:" and "Subject:" header lines at the start, then treats
// everything after the headers (and any blank separator) as the body.
func parseDraft(raw string) *Draft {
	d := &Draft{Raw: raw}

	lines := strings.Split(raw, "\n")
	headerEnd := 0

	// Scan for known header lines at the top of the output.
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "To:"):
			d.To = strings.TrimSpace(strings.TrimPrefix(trimmed, "To:"))
			headerEnd = i + 1
		case strings.HasPrefix(trimmed, "Subject:"):
			d.Subject = strings.TrimSpace(strings.TrimPrefix(trimmed, "Subject:"))
			headerEnd = i + 1
		case trimmed == "":
			// Skip blank lines between/after headers
			if headerEnd > 0 {
				headerEnd = i + 1
			}
		default:
			// First non-header, non-blank line — stop scanning
			goto done
		}
	}
done:

	// If no headers were found, the entire output is the body.
	if d.To == "" && d.Subject == "" {
		headerEnd = 0
	}

	d.Body = strings.TrimSpace(strings.Join(lines[headerEnd:], "\n"))
	return d
}

// Package synthesis defines the synthesizer interface and passthrough implementation.
package synthesis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jcadam/burrow/pkg/services"
)

// Synthesizer takes collected service results and produces a markdown report.
type Synthesizer interface {
	Synthesize(ctx context.Context, title string, systemPrompt string, results []*services.Result) (string, error)
}

// Provider is the interface for LLM backends.
type Provider interface {
	Complete(ctx context.Context, systemPrompt string, userPrompt string) (string, error)
}

// PassthroughSynthesizer formats raw results as structured markdown without an LLM.
type PassthroughSynthesizer struct{}

// NewPassthroughSynthesizer creates a synthesizer that formats results directly.
func NewPassthroughSynthesizer() *PassthroughSynthesizer {
	return &PassthroughSynthesizer{}
}

// Synthesize produces a markdown report from raw service results.
func (p *PassthroughSynthesizer) Synthesize(_ context.Context, title string, _ string, results []*services.Result) (string, error) {
	var b strings.Builder

	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")

	b.WriteString("*Generated: ")
	b.WriteString(time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	b.WriteString("*\n\n")

	successCount := 0
	errorCount := 0
	for _, r := range results {
		if r.Error != "" {
			errorCount++
		} else {
			successCount++
		}
	}

	b.WriteString(fmt.Sprintf("**Sources queried:** %d | **Successful:** %d | **Errors:** %d\n\n",
		len(results), successCount, errorCount))
	b.WriteString("---\n\n")

	for _, r := range results {
		b.WriteString("## ")
		b.WriteString(r.Service)
		b.WriteString(" — ")
		b.WriteString(r.Tool)
		b.WriteString("\n\n")

		if r.Error != "" {
			b.WriteString(fmt.Sprintf("> **Error:** %s\n\n", r.Error))
			if len(r.Data) > 0 {
				b.WriteString("```\n")
				b.WriteString(string(r.Data))
				b.WriteString("\n```\n\n")
			}
			continue
		}

		b.WriteString("```\n")
		b.WriteString(string(r.Data))
		b.WriteString("\n```\n\n")
	}

	return b.String(), nil
}

// LLMSynthesizer uses an LLM provider for synthesis.
type LLMSynthesizer struct {
	provider         Provider
	stripAttribution bool
}

// NewLLMSynthesizer creates a synthesizer backed by an LLM provider.
// When stripAttribution is true, service names are replaced with generic labels
// before sending data to the provider (required for remote LLMs per spec).
func NewLLMSynthesizer(provider Provider, stripAttribution bool) *LLMSynthesizer {
	return &LLMSynthesizer{provider: provider, stripAttribution: stripAttribution}
}

// Synthesize sends collected results through the LLM for synthesis.
func (l *LLMSynthesizer) Synthesize(ctx context.Context, title string, systemPrompt string, results []*services.Result) (string, error) {
	var userPrompt strings.Builder
	userPrompt.WriteString("Generate a report titled: ")
	userPrompt.WriteString(title)
	userPrompt.WriteString("\n\nSource data:\n\n")

	for i, r := range results {
		label := r.Service + " — " + r.Tool
		if l.stripAttribution {
			label = fmt.Sprintf("Source %d", i+1)
		}

		userPrompt.WriteString("### ")
		userPrompt.WriteString(label)
		userPrompt.WriteString("\n")
		if r.Error != "" {
			errMsg := r.Error
			if l.stripAttribution {
				errMsg = stripServiceNames(errMsg, results)
			}
			userPrompt.WriteString("Error: ")
			userPrompt.WriteString(errMsg)
			userPrompt.WriteString("\n")
		} else {
			userPrompt.WriteString(string(r.Data))
			userPrompt.WriteString("\n")
		}
		userPrompt.WriteString("\n")
	}

	return l.provider.Complete(ctx, systemPrompt, userPrompt.String())
}

// stripServiceNames replaces any service name found in text with a generic placeholder.
func stripServiceNames(text string, results []*services.Result) string {
	seen := make(map[string]bool)
	for _, r := range results {
		if r.Service != "" && !seen[r.Service] {
			text = strings.ReplaceAll(text, r.Service, "[service]")
			seen[r.Service] = true
		}
	}
	return text
}

// Package synthesis defines the synthesizer interface and passthrough implementation.
package synthesis

import (
	"context"
	"fmt"
	"regexp"
	"sort"
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

// Shared prompt instructions used by both single-stage and multi-stage synthesis paths.
const (
	staticDocumentInstruction = "Output format: This is a static report document, not a conversation. " +
		"Begin your response with the report content immediately — no preamble, no reasoning, no planning. " +
		"Do NOT write phrases like \"The user wants\", \"I should\", \"Let me analyze\", or \"Here is the report\". " +
		"Never include greetings, sign-offs, offers to help, or closing phrases like " +
		"\"Let me know if you have questions\" or \"Reply to refine.\" " +
		"End the report with the final section's content."

	urlInstruction = "When source data contains URLs or link fields (\"link\", \"url\", \"href\"), " +
		"use the exact URL from the data as-is — do not construct, guess, or modify URLs. " +
		"Format them as markdown links: [Title](https://example.com). " +
		"Every news item, paper, or article with a URL must have a clickable link in the report. " +
		"Never break a URL across lines."

	missingDataInstruction = "When source data has missing or incomplete fields, always analyze what IS present. " +
		"Never skip a section or declare \"none included\" because some records lack a field. " +
		"Present the available data, note any limitations briefly in parentheses, and move on."

	// Compact variants for local models — fewer tokens, same rules.
	localStaticDocumentInstruction = "You write factual report documents. " +
		"Start immediately with content. No chat, no reasoning, no greetings, no closings."

	localInstructions = "\n\nRULES:\n" +
		"1. Start with the report immediately. No preamble, no reasoning.\n" +
		"2. Use exact URLs from the data as markdown links: [Title](URL)\n" +
		"3. Include all specific numbers, dates, and names from the data.\n" +
		"4. If data is missing or incomplete, report what IS present.\n" +
		"5. No sign-offs or offers to help. End with the last section.\n"
)

// LLMSynthesizer uses an LLM provider for synthesis.
type LLMSynthesizer struct {
	provider         Provider
	stripAttribution bool
	localModel       bool
	multiStage       MultiStageConfig
}

// NewLLMSynthesizer creates a synthesizer backed by an LLM provider.
// When stripAttribution is true, service names are replaced with generic labels
// before sending data to the provider (required for remote LLMs per spec).
func NewLLMSynthesizer(provider Provider, stripAttribution bool) *LLMSynthesizer {
	return &LLMSynthesizer{provider: provider, stripAttribution: stripAttribution}
}

// SetLocalModel enables compact prompt variants optimized for smaller local models.
func (l *LLMSynthesizer) SetLocalModel(local bool) {
	l.localModel = local
}

// SetMultiStage configures multi-stage synthesis behavior.
func (l *LLMSynthesizer) SetMultiStage(cfg MultiStageConfig) {
	l.multiStage = cfg
}

// Synthesize sends collected results through the LLM for synthesis.
// It routes to single-stage or multi-stage based on configuration and data size.
func (l *LLMSynthesizer) Synthesize(ctx context.Context, title string, systemPrompt string, results []*services.Result) (string, error) {
	if l.shouldMultiStage(results) {
		return l.synthesizeMultiStage(ctx, title, systemPrompt, results)
	}
	return l.synthesizeSingle(ctx, title, systemPrompt, results)
}

// synthesizeSingle is the original single-call synthesis path.
func (l *LLMSynthesizer) synthesizeSingle(ctx context.Context, title string, systemPrompt string, results []*services.Result) (string, error) {
	// Append static-document instruction to system prompt so it takes
	// precedence over the LLM's conversational training.
	fullSystem := systemPrompt
	if fullSystem != "" {
		fullSystem += "\n\n"
	}
	if l.localModel {
		fullSystem += localStaticDocumentInstruction
	} else {
		fullSystem += staticDocumentInstruction
	}

	var userPrompt strings.Builder
	userPrompt.WriteString("Generate a report titled: ")
	userPrompt.WriteString(title)
	userPrompt.WriteString("\n\nSource data:\n\n")

	for i, r := range results {
		label := r.Service + " — " + r.Tool
		if r.ContextLabel != "" {
			label = r.ContextLabel
		}
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
			data := string(r.Data)
			if l.stripAttribution {
				data = stripServiceNames(data, results)
			}
			userPrompt.WriteString(data)
			userPrompt.WriteString("\n")
		}
		userPrompt.WriteString("\n")
	}

	if l.localModel {
		userPrompt.WriteString(localInstructions)
	} else {
		userPrompt.WriteString("\n---\n")
		userPrompt.WriteString(urlInstruction)
		userPrompt.WriteString("\n")

		userPrompt.WriteString("\n---\n")
		userPrompt.WriteString(missingDataInstruction)
		userPrompt.WriteString("\n")

		userPrompt.WriteString("\n---\nBegin with report content immediately. No preamble, no reasoning, no conversational closing.\n")
	}

	result, err := l.provider.Complete(ctx, fullSystem, userPrompt.String())
	if err != nil {
		return "", err
	}
	return postProcess(result), nil
}

// brokenURLPattern matches markdown link URLs that contain newlines: ](url\nrest)
var brokenURLPattern = regexp.MustCompile(`\]\(([^)]*\n[^)]*)\)`)

// repairBrokenURLs fixes markdown links where the URL was wrapped across lines
// by the LLM. It strips all whitespace characters from inside ](...) when a
// newline is present. Only targets markdown links — bare URLs are left alone.
func repairBrokenURLs(text string) string {
	return brokenURLPattern.ReplaceAllStringFunc(text, func(match string) string {
		inner := match[2 : len(match)-1] // strip "](" and ")"
		var cleaned strings.Builder
		for _, r := range inner {
			if r != '\n' && r != '\r' && r != ' ' && r != '\t' {
				cleaned.WriteRune(r)
			}
		}
		return "](" + cleaned.String() + ")"
	})
}

// conversationalClosings are substring patterns matched (case-insensitive) against
// trailing lines of LLM output to strip chatbot-style sign-offs from reports.
var conversationalClosings = []string{
	"let me know",
	"feel free to",
	"don't hesitate",
	"do not hesitate",
	"happy to help",
	"hope this helps",
	"if you need anything",
	"if you have any questions",
	"if you have questions",
	"reply to refine",
	"questions? reply",
	"reach out if",
	"glad to help",
	"here to help",
	"want me to",
	"shall i",
	"i can also",
	"i'd be happy",
	"i would be happy",
}

// trimConversationalClosing removes chatbot-style closing lines from the end of
// LLM output. It walks backwards from the last non-empty line (up to 5 non-empty
// lines) and strips lines that match known conversational patterns, as well as
// trailing separators (---) and blank lines. Lines starting with > or ` are treated
// as real content and stop the scan.
func trimConversationalClosing(text string) string {
	trimmed := strings.TrimRight(text, " \t\n\r")
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")

	cutIndex := len(lines) // index of first line to remove
	scanned := 0           // count of non-empty lines examined

	for i := len(lines) - 1; i >= 0 && scanned < 5; i-- {
		line := strings.TrimSpace(lines[i])

		// Empty lines and horizontal rules between closings get removed too.
		if line == "" || line == "---" {
			cutIndex = i
			continue
		}

		// Blockquotes and code fences are real content — stop.
		if strings.HasPrefix(line, ">") || strings.HasPrefix(line, "`") {
			break
		}

		scanned++
		lower := strings.ToLower(line)
		matched := false
		for _, pat := range conversationalClosings {
			if strings.Contains(lower, pat) {
				matched = true
				break
			}
		}
		if !matched {
			break
		}
		cutIndex = i
	}

	result := strings.Join(lines[:cutIndex], "\n")
	return strings.TrimRight(result, " \t\n\r") + "\n"
}

// postProcess applies all output cleaning steps to LLM output.
func postProcess(text string) string {
	text = stripThinkTags(text)
	text = trimThinkingPreamble(text)
	text = repairBrokenURLs(text)
	text = trimConversationalClosing(text)
	return text
}

// thinkTagPattern matches <think>...</think> blocks (case-insensitive, multiline).
var thinkTagPattern = regexp.MustCompile(`(?is)<think>.*?</think>\s*`)

// stripThinkTags removes <think>...</think> blocks emitted by reasoning models
// (DeepSeek-R1, QwQ, etc.). Always enabled — a no-op on clean output.
func stripThinkTags(text string) string {
	return thinkTagPattern.ReplaceAllString(text, "")
}

// thinkingPreamblePatterns are line-start patterns that indicate the LLM is
// "thinking out loud" rather than producing report content.
var thinkingPreamblePatterns = []string{
	"the user wants",
	"the user is asking",
	"i should",
	"i need to",
	"i will",
	"i'll",
	"let me analyze",
	"let me review",
	"let me examine",
	"let me look",
	"let me start",
	"let me create",
	"let me generate",
	"let me summarize",
	"let me compile",
	"let me put together",
	"here is the report",
	"here's the report",
	"here is a report",
	"here's a report",
	"here is your report",
	"here's your report",
	"below is the report",
	"below is a report",
	"i've compiled",
	"i have compiled",
	"i've analyzed",
	"i have analyzed",
	"i've reviewed",
	"i have reviewed",
	"based on the data provided",
	"based on the information",
	"based on the source data",
	"looking at the data",
	"looking at the source",
	"analyzing the data",
	"after reviewing",
	"after analyzing",
	"okay, ",
	"sure, ",
	"certainly, ",
	"alright, ",
}

// trimThinkingPreamble strips "thinking out loud" lines from the start of LLM
// output. Walks forward, removing lines that match known thinking patterns or
// are empty, until the first markdown heading or first non-matching non-empty line.
// Always enabled — a no-op on clean output that starts with content.
func trimThinkingPreamble(text string) string {
	lines := strings.Split(text, "\n")
	startIdx := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Empty lines at the start get skipped.
		if trimmed == "" {
			startIdx = i + 1
			continue
		}

		// A markdown heading is real content — stop.
		if strings.HasPrefix(trimmed, "#") {
			startIdx = i
			break
		}

		// Check if this line matches a thinking pattern.
		lower := strings.ToLower(trimmed)
		matched := false
		for _, pat := range thinkingPreamblePatterns {
			if strings.HasPrefix(lower, pat) {
				matched = true
				break
			}
		}
		if matched {
			startIdx = i + 1
			continue
		}

		// Non-empty, non-matching, non-heading — real content, stop.
		startIdx = i
		break
	}

	if startIdx >= len(lines) {
		return text // all lines were preamble — return original to avoid empty output
	}
	return strings.Join(lines[startIdx:], "\n")
}

// stripServiceNames replaces any service name found in text with a generic placeholder.
// Names are sorted longest-first to prevent substring corruption (e.g., "news-api"
// must be replaced before "news" to avoid producing "[service]-api").
func stripServiceNames(text string, results []*services.Result) string {
	var names []string
	seen := make(map[string]bool)
	for _, r := range results {
		if len(r.Service) < 3 || seen[r.Service] {
			continue
		}
		seen[r.Service] = true
		names = append(names, r.Service)
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j])
	})
	for _, name := range names {
		text = strings.ReplaceAll(text, name, "[service]")
	}
	return text
}

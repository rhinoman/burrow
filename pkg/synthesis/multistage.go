package synthesis

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jcadam/burrow/pkg/services"
)

// MultiStageConfig controls when and how multi-stage synthesis is used.
type MultiStageConfig struct {
	Strategy        string // auto | single | multi-stage
	SummaryMaxWords int    // target words per stage 1 summary (default: 500)
	ThresholdBytes  int    // auto-trigger threshold (default: 16384)
	MaxSourceWords  int    // max words per source before chunking (default: 10000)
	Concurrency     int    // max concurrent stage 1 LLM calls (default: 1)
	ContextWindow   int    // model context window in tokens; used to derive MaxSourceWords when 0
}

const (
	defaultSummaryMaxWords  = 500
	defaultThresholdBytes   = 16384
	defaultMaxSourceWords   = 10000
)

// stage1SystemPrompt is the system prompt for per-source summarization calls.
const stage1SystemPrompt = "You are a data summarization assistant. Extract the key facts, figures, and notable items " +
	"from the source data below. Preserve all URLs, dates, numbers, and proper nouns exactly. " +
	"Remove redundant or boilerplate content. Output a concise summary in plain text. " +
	"Begin immediately with the summary — no preamble."

// summaryMaxWords returns the configured summary word target or the default.
func (c MultiStageConfig) summaryMaxWords() int {
	if c.SummaryMaxWords > 0 {
		return c.SummaryMaxWords
	}
	return defaultSummaryMaxWords
}

// thresholdBytes returns the configured threshold or the default.
func (c MultiStageConfig) thresholdBytes() int {
	if c.ThresholdBytes > 0 {
		return c.ThresholdBytes
	}
	return defaultThresholdBytes
}

// autoThresholdBytes returns the threshold for auto strategy decisions.
// It scales with the model's context window so large-context cloud models
// use single-stage (better cross-source context) while small-context local
// models use multi-stage (avoids overflow).
//
// Math: 1 token ≈ 4 bytes (conservative for English/JSON mix).
// Use 50% of context window for source data, leaving room for prompts + output.
//
// Examples:
//   - Haiku 200K tokens: 200000 * 0.5 * 4 = 400KB threshold
//   - Local qwen3-32b 65K: 65536 * 0.5 * 4 = 128KB threshold
//   - Default local 8K: 8192 * 0.5 * 4 = 16KB (matches defaultThresholdBytes)
func (c MultiStageConfig) autoThresholdBytes() int {
	if c.ThresholdBytes > 0 {
		return c.ThresholdBytes // explicit override always wins
	}
	if c.ContextWindow > 0 {
		return int(float64(c.ContextWindow) * 0.5 * 4)
	}
	return defaultThresholdBytes
}

// maxSourceWords returns the configured max source words or the default.
// When MaxSourceWords is 0 and ContextWindow is set, derives from context window
// using 40% of tokens (reserving ~60% for system prompt + response).
func (c MultiStageConfig) maxSourceWords() int {
	if c.MaxSourceWords > 0 {
		return c.MaxSourceWords
	}
	if c.ContextWindow > 0 {
		return int(float64(c.ContextWindow) * 0.4)
	}
	return defaultMaxSourceWords
}

// concurrency returns the configured stage 1 concurrency or the default (1).
func (c MultiStageConfig) concurrency() int {
	if c.Concurrency > 0 {
		return c.Concurrency
	}
	return 1
}

// shouldMultiStage decides whether to use multi-stage synthesis.
func (l *LLMSynthesizer) shouldMultiStage(results []*services.Result) bool {
	strategy := l.multiStage.Strategy
	switch strategy {
	case "single":
		return false
	case "multi-stage":
		return true
	default: // "auto" or ""
		total := 0
		for _, r := range results {
			total += len(r.Data)
		}
		return total > l.multiStage.autoThresholdBytes()
	}
}

// sourceSummary holds the result of a stage 1 summarization call.
type sourceSummary struct {
	label   string
	summary string
	err     error
}

// summarizeChunk makes a single stage 1 LLM call to summarize one chunk of data.
func (l *LLMSynthesizer) summarizeChunk(ctx context.Context, label, data, priorities string) sourceSummary {
	var userPrompt strings.Builder
	userPrompt.WriteString("Context label: ")
	userPrompt.WriteString(label)
	userPrompt.WriteString("\n\n")

	if priorities != "" {
		userPrompt.WriteString("User priorities: ")
		userPrompt.WriteString(priorities)
		userPrompt.WriteString("\n\n")
	}

	userPrompt.WriteString("Source data:\n")
	userPrompt.WriteString(data)
	userPrompt.WriteString("\n\n")
	userPrompt.WriteString(fmt.Sprintf("Target length: approximately %d words.", l.multiStage.summaryMaxWords()))

	summary, err := l.provider.Complete(ctx, stage1SystemPrompt, userPrompt.String())
	if err != nil {
		return sourceSummary{label: label, err: err}
	}
	return sourceSummary{label: label, summary: truncateSummary(summary, l.multiStage.summaryMaxWords()*2)}
}

// summarizeSource summarizes one source, chunking if the data exceeds maxSourceWords.
func (l *LLMSynthesizer) summarizeSource(ctx context.Context, idx int, r *services.Result, priorities string) sourceSummary {
	label := r.Service + " — " + r.Tool
	if r.ContextLabel != "" {
		label = r.ContextLabel
	}
	if l.stripAttribution {
		label = fmt.Sprintf("Source %d", idx+1)
	}

	if r.Error != "" {
		return sourceSummary{label: label, summary: "Error: " + r.Error}
	}
	if len(r.Data) == 0 {
		return sourceSummary{label: label, summary: "(no data)"}
	}

	data := string(r.Data)
	if l.stripAttribution {
		data = stripServiceNames(data, []*services.Result{r})
	}

	// If data fits within maxSourceWords, single LLM call
	if countWords(data) <= l.multiStage.maxSourceWords() {
		return l.summarizeChunk(ctx, label, data, priorities)
	}

	// Data too large — chunk it and summarize each chunk sequentially
	chunks := splitIntoChunks(data, l.multiStage.maxSourceWords())

	var summaries []string
	for i, chunk := range chunks {
		chunkLabel := fmt.Sprintf("%s (part %d/%d)", label, i+1, len(chunks))
		result := l.summarizeChunk(ctx, chunkLabel, chunk, priorities)
		if result.err != nil {
			return sourceSummary{label: label, err: result.err}
		}
		summaries = append(summaries, result.summary)
	}

	merged := strings.Join(summaries, "\n\n")
	return sourceSummary{label: label, summary: truncateSummary(merged, l.multiStage.summaryMaxWords()*2)}
}

// runStage1 executes all stage 1 calls concurrently, bounded by a semaphore.
// Concurrency defaults to 1 (sequential) to avoid overwhelming local LLM servers.
func (l *LLMSynthesizer) runStage1(ctx context.Context, results []*services.Result, priorities string) []sourceSummary {
	summaries := make([]sourceSummary, len(results))
	var wg sync.WaitGroup
	sem := make(chan struct{}, l.multiStage.concurrency())

	for i, r := range results {
		wg.Add(1)
		go func(idx int, r *services.Result) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			summaries[idx] = l.summarizeSource(ctx, idx, r, priorities)
		}(i, r)
	}

	wg.Wait()
	return summaries
}

// synthesizeMultiStage runs the two-stage pipeline.
func (l *LLMSynthesizer) synthesizeMultiStage(ctx context.Context, title string, systemPrompt string, results []*services.Result) (string, error) {
	priorities := extractPriorities(systemPrompt)

	// Stage 1: per-source summarization (parallel)
	summaries := l.runStage1(ctx, results, priorities)

	// For failed summaries, fall back to truncated raw data
	for i, s := range summaries {
		if s.err != nil && i < len(results) {
			raw := truncateRawFallback(string(results[i].Data), l.multiStage.summaryMaxWords()*3)
			summaries[i] = sourceSummary{
				label:   s.label,
				summary: raw,
			}
		}
	}

	// Bound summaries so stage 2 prompt fits within context window.
	summaries = l.boundStage2Summaries(summaries)

	// Stage 2: assembly
	userPrompt := l.assembleStage2Prompt(title, summaries)

	// Build system prompt with static-document instruction (same as single-stage)
	fullSystem := systemPrompt
	if fullSystem != "" {
		fullSystem += "\n\n"
	}
	fullSystem += staticDocumentInstruction

	result, err := l.provider.Complete(ctx, fullSystem, userPrompt)
	if err != nil {
		return "", err
	}
	return postProcess(result), nil
}

// boundStage2Summaries truncates summaries so the stage 2 prompt fits within
// the model's context window. Uses 60% of the context window (in bytes, at
// ~4 bytes/token) for summaries, leaving room for the system prompt and output.
// When total summary text exceeds the budget, each summary is proportionally
// truncated to fit.
func (l *LLMSynthesizer) boundStage2Summaries(summaries []sourceSummary) []sourceSummary {
	if l.multiStage.ContextWindow <= 0 {
		return summaries
	}

	// 60% of context for summaries (rest for system prompt + generation)
	budgetBytes := int(float64(l.multiStage.ContextWindow) * 0.6 * 4)

	totalBytes := 0
	for _, s := range summaries {
		totalBytes += len(s.summary)
	}

	if totalBytes <= budgetBytes {
		return summaries
	}

	// Proportionally truncate each summary
	ratio := float64(budgetBytes) / float64(totalBytes)
	bounded := make([]sourceSummary, len(summaries))
	for i, s := range summaries {
		maxWords := int(float64(countWords(s.summary)) * ratio)
		if maxWords < 50 {
			maxWords = 50
		}
		bounded[i] = sourceSummary{
			label:   s.label,
			summary: truncateSummary(s.summary, maxWords),
		}
	}
	return bounded
}

// assembleStage2Prompt builds the user prompt for the final assembly call.
func (l *LLMSynthesizer) assembleStage2Prompt(title string, summaries []sourceSummary) string {
	var b strings.Builder
	b.WriteString("Generate a report titled: ")
	b.WriteString(title)
	b.WriteString("\n\nPre-summarized source data extracts:\n\n")

	for _, s := range summaries {
		b.WriteString("### ")
		b.WriteString(s.label)
		b.WriteString("\n")
		b.WriteString(s.summary)
		b.WriteString("\n\n")
	}

	b.WriteString("\n---\n")
	b.WriteString(urlInstruction)
	b.WriteString("\n")

	b.WriteString("\n---\n")
	b.WriteString(missingDataInstruction)
	b.WriteString("\n")

	b.WriteString("\n---\nBegin with report content immediately. No preamble, no reasoning, no conversational closing.\n")

	return b.String()
}

// extractPriorities returns a truncated excerpt of the system prompt for use
// in stage 1 calls. This gives the summarizer enough context to filter
// intelligently without sending the full prompt.
func extractPriorities(systemPrompt string) string {
	const maxChars = 300
	if len(systemPrompt) <= maxChars {
		return systemPrompt
	}
	// Truncate at a word boundary
	truncated := systemPrompt[:maxChars]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxChars/2 {
		truncated = truncated[:lastSpace]
	}
	return truncated + "..."
}

// truncateSummary ensures a summary doesn't exceed a word limit.
func truncateSummary(text string, maxWords int) string {
	words := strings.Fields(text)
	if len(words) <= maxWords {
		return text
	}
	return strings.Join(words[:maxWords], " ") + "..."
}

// truncateRawFallback truncates raw data for use as a fallback when stage 1 fails.
func truncateRawFallback(data string, maxWords int) string {
	words := strings.Fields(data)
	if len(words) <= maxWords {
		return data
	}
	return strings.Join(words[:maxWords], " ") + "\n[... truncated ...]"
}

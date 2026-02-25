package synthesis

import (
	"encoding/json"
	"strings"
)

// splitIntoChunks splits data into chunks of approximately maxWords words each,
// preserving record boundaries where possible. It tries strategies in order:
// 1. Top-level JSON array — split on array elements
// 2. JSON object with array field — split on the largest array field's elements
// 3. Line-based — split on double-newline paragraph boundaries
// 4. Word-based fallback — split into roughly equal word-count chunks
func splitIntoChunks(data string, maxWords int) []string {
	if countWords(data) <= maxWords {
		return []string{data}
	}

	// Strategy 1: top-level JSON array
	if chunks := splitJSONArray(data, maxWords); chunks != nil {
		return chunks
	}

	// Strategy 2: JSON object with array field
	if chunks := splitJSONObjectWithArray(data, maxWords); chunks != nil {
		return chunks
	}

	// Strategy 3: line-based (paragraph boundaries)
	if chunks := splitLineBased(data, maxWords); chunks != nil {
		return chunks
	}

	// Strategy 4: word-based fallback
	return splitWords(data, maxWords)
}

// splitJSONArray handles top-level JSON arrays like [{...}, {...}].
// Returns nil if data is not a JSON array.
func splitJSONArray(data string, maxWords int) []string {
	trimmed := strings.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil
	}

	var elements []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &elements); err != nil {
		return nil
	}
	if len(elements) <= 1 {
		return nil
	}

	records := make([]string, len(elements))
	for i, el := range elements {
		records[i] = string(el)
	}

	groups := groupRecords(records, maxWords)

	// Reassemble each group as a valid JSON array
	chunks := make([]string, len(groups))
	for i, g := range groups {
		chunks[i] = "[" + g + "]"
	}
	return chunks
}

// splitJSONObjectWithArray handles JSON objects like {"results": [...], "meta": ...}.
// It finds the largest array-valued field, splits its elements, and preserves
// non-array fields in each chunk. Returns nil if data is not a JSON object
// with an array field.
func splitJSONObjectWithArray(data string, maxWords int) []string {
	trimmed := strings.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return nil
	}

	// Find the largest array field
	var largestKey string
	var largestArray []json.RawMessage
	for key, val := range obj {
		var arr []json.RawMessage
		if err := json.Unmarshal(val, &arr); err != nil {
			continue
		}
		if len(arr) > len(largestArray) {
			largestKey = key
			largestArray = arr
		}
	}

	if len(largestArray) <= 1 {
		return nil
	}

	records := make([]string, len(largestArray))
	for i, el := range largestArray {
		records[i] = string(el)
	}

	groups := groupRecords(records, maxWords)
	if len(groups) <= 1 {
		return nil
	}

	// Build the non-array context fields
	contextFields := make(map[string]json.RawMessage)
	for key, val := range obj {
		if key != largestKey {
			contextFields[key] = val
		}
	}

	// Reassemble each group with context fields preserved
	chunks := make([]string, len(groups))
	for i, g := range groups {
		// Build a new JSON object: context fields + chunked array
		parts := make(map[string]json.RawMessage)
		for k, v := range contextFields {
			parts[k] = v
		}
		parts[largestKey] = json.RawMessage("[" + g + "]")

		b, err := json.Marshal(parts)
		if err != nil {
			// Shouldn't happen — fall through to next strategy
			return nil
		}
		chunks[i] = string(b)
	}
	return chunks
}

// splitLineBased splits on double-newline paragraph boundaries.
// Returns nil if the data has no paragraph breaks.
func splitLineBased(data string, maxWords int) []string {
	paragraphs := strings.Split(data, "\n\n")
	if len(paragraphs) <= 1 {
		return nil
	}

	// Filter empty paragraphs
	var nonEmpty []string
	for _, p := range paragraphs {
		if strings.TrimSpace(p) != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) <= 1 {
		return nil
	}

	groups := groupRecords(nonEmpty, maxWords)
	if len(groups) <= 1 {
		return nil
	}

	return groups
}

// splitWords splits data into chunks of approximately maxWords words each.
// This is the last-resort fallback for unstructured text.
func splitWords(data string, maxWords int) []string {
	words := strings.Fields(data)
	if len(words) <= maxWords {
		return []string{data}
	}

	var chunks []string
	for i := 0; i < len(words); i += maxWords {
		end := i + maxWords
		if end > len(words) {
			end = len(words)
		}
		chunks = append(chunks, strings.Join(words[i:end], " "))
	}
	return chunks
}

// groupRecords packs records into chunks where each chunk's word count is
// approximately ≤ maxWords. A single record exceeding maxWords becomes its
// own chunk (stage 1 error handling will deal with it).
// For JSON records, groups are joined with commas. For text records, groups
// are joined with double-newlines.
func groupRecords(records []string, maxWords int) []string {
	if len(records) == 0 {
		return nil
	}

	// Detect if records look like JSON (for join separator)
	isJSON := false
	for _, r := range records {
		trimmed := strings.TrimSpace(r)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			isJSON = true
			break
		}
	}

	separator := "\n\n"
	if isJSON {
		separator = ","
	}

	var chunks []string
	var current []string
	currentWords := 0

	for _, rec := range records {
		recWords := countWords(rec)

		// If adding this record would exceed maxWords and we already have records,
		// flush the current chunk
		if currentWords > 0 && currentWords+recWords > maxWords {
			chunks = append(chunks, strings.Join(current, separator))
			current = nil
			currentWords = 0
		}

		current = append(current, rec)
		currentWords += recWords
	}

	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, separator))
	}

	return chunks
}

// countWords returns the number of whitespace-delimited words in s.
func countWords(s string) int {
	return len(strings.Fields(s))
}

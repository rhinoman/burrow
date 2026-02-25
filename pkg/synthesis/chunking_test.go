package synthesis

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSplitJSONArray(t *testing.T) {
	// 3 elements, maxWords small enough to force splitting
	data := `[{"id":1,"title":"First opportunity with some extra words"},{"id":2,"title":"Second opportunity with more details"},{"id":3,"title":"Third opportunity description here"}]`

	chunks := splitIntoChunks(data, 5)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// Each chunk should be valid JSON array
	for i, chunk := range chunks {
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(chunk), &arr); err != nil {
			t.Errorf("chunk %d is not valid JSON array: %v\nchunk: %s", i, err, chunk)
		}
	}
}

func TestSplitJSONObjectWithArray(t *testing.T) {
	data := `{"meta":{"page":1},"results":[{"id":1,"name":"Alpha bravo charlie delta"},{"id":2,"name":"Echo foxtrot golf hotel"},{"id":3,"name":"India juliet kilo lima"}]}`

	chunks := splitIntoChunks(data, 6)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// Each chunk should be valid JSON object with meta preserved
	for i, chunk := range chunks {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(chunk), &obj); err != nil {
			t.Errorf("chunk %d is not valid JSON object: %v", i, err)
			continue
		}
		if _, ok := obj["meta"]; !ok {
			t.Errorf("chunk %d missing 'meta' context field", i)
		}
		if _, ok := obj["results"]; !ok {
			t.Errorf("chunk %d missing 'results' field", i)
		}
		// Verify results is still a valid JSON array
		var arr []json.RawMessage
		if err := json.Unmarshal(obj["results"], &arr); err != nil {
			t.Errorf("chunk %d 'results' is not valid JSON array: %v", i, err)
		}
	}
}

func TestSplitLineBased(t *testing.T) {
	paragraphs := []string{
		"First paragraph with several words in it here.",
		"Second paragraph also containing multiple words.",
		"Third paragraph has a few words too for splitting.",
	}
	data := strings.Join(paragraphs, "\n\n")

	chunks := splitIntoChunks(data, 6)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks from line-based split, got %d", len(chunks))
	}

	// Verify all original content is present
	all := strings.Join(chunks, " ")
	for _, p := range paragraphs {
		if !strings.Contains(all, strings.TrimSpace(p)) {
			t.Errorf("paragraph not found in chunks: %q", p)
		}
	}
}

func TestSplitWordFallback(t *testing.T) {
	words := make([]string, 30)
	for i := range words {
		words[i] = "word"
	}
	data := strings.Join(words, " ")

	chunks := splitIntoChunks(data, 10)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks from 30 words at maxWords=10, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		wc := countWords(chunk)
		if wc > 10 {
			t.Errorf("chunk %d has %d words, expected ≤10", i, wc)
		}
	}
}

func TestSplitSmallData(t *testing.T) {
	data := "small amount of data"
	chunks := splitIntoChunks(data, 100)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for small data, got %d", len(chunks))
	}
	if chunks[0] != data {
		t.Errorf("expected unchanged data, got %q", chunks[0])
	}
}

func TestGroupRecords(t *testing.T) {
	records := []string{
		"one two",
		"three four",
		"five six seven eight nine ten",
		"eleven twelve",
	}

	groups := groupRecords(records, 5)

	// First group: "one two" + "three four" = 4 words
	// Second group: "five six seven eight nine ten" = 6 words (oversized single record, own chunk)
	// Third group: "eleven twelve" = 2 words
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(groups), groups)
	}

	// Oversized single record should be its own chunk
	if !strings.Contains(groups[1], "five six seven eight nine ten") {
		t.Errorf("expected oversized record in its own group, got %q", groups[1])
	}
}

func TestGroupRecordsEmpty(t *testing.T) {
	groups := groupRecords(nil, 10)
	if groups != nil {
		t.Errorf("expected nil for empty records, got %v", groups)
	}
}

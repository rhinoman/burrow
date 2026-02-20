package configure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SpecFormat describes the detected format of an API specification.
type SpecFormat string

const (
	SpecFormatOpenAPIJSON SpecFormat = "openapi-json"
	SpecFormatOpenAPIYAML SpecFormat = "openapi-yaml"
	SpecFormatHTML        SpecFormat = "html"
	SpecFormatUnknown     SpecFormat = "unknown"
)

const maxSpecBytes = 100_000 // ~100KB limit for spec content injected into LLM prompt
const maxReadBytes = 1_000_000 // 1MB cap on response body to prevent OOM

// FetchedSpec holds the fetched and processed API specification content.
type FetchedSpec struct {
	URL     string
	Format  SpecFormat
	Content string // Possibly truncated spec content for LLM consumption
	Error   string // Non-empty if fetch failed (prevents retry)
}

// FetchSpec retrieves an API specification from a URL and prepares it for LLM consumption.
// Uses its own http.Client with 30s timeout. Reads at most 1MB from the response to
// prevent OOM, then truncates to maxSpecBytes if still too large.
func FetchSpec(ctx context.Context, specURL string) (*FetchedSpec, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, specURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for spec %q: %w", specURL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching spec %q: %w", specURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching spec %q: HTTP %d", specURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReadBytes))
	if err != nil {
		return nil, fmt.Errorf("reading spec %q: %w", specURL, err)
	}

	format := detectFormat(resp.Header.Get("Content-Type"), body)
	content := string(body)

	// Truncate to maxSpecBytes using rune conversion for UTF-8 safety.
	runes := []rune(content)
	if len(runes) > maxSpecBytes {
		content = string(runes[:maxSpecBytes]) + "\n\n[Spec truncated — showing first ~100KB]"
	}

	return &FetchedSpec{
		URL:     specURL,
		Format:  format,
		Content: content,
	}, nil
}

// detectFormat determines the spec format from Content-Type header and body inspection.
func detectFormat(contentType string, body []byte) SpecFormat {
	ct := strings.ToLower(contentType)

	// Check Content-Type first.
	if strings.Contains(ct, "application/json") || strings.Contains(ct, "application/vnd.oai.openapi+json") {
		if looksLikeOpenAPIJSON(body) {
			return SpecFormatOpenAPIJSON
		}
	}
	if strings.Contains(ct, "text/yaml") || strings.Contains(ct, "application/yaml") ||
		strings.Contains(ct, "application/x-yaml") || strings.Contains(ct, "application/vnd.oai.openapi") {
		if looksLikeOpenAPIYAML(body) {
			return SpecFormatOpenAPIYAML
		}
	}
	if strings.Contains(ct, "text/html") {
		return SpecFormatHTML
	}

	// Fall back to body inspection.
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) == 0 {
		return SpecFormatUnknown
	}

	if trimmed[0] == '{' && looksLikeOpenAPIJSON(body) {
		return SpecFormatOpenAPIJSON
	}
	if looksLikeOpenAPIYAML(body) {
		return SpecFormatOpenAPIYAML
	}
	if trimmed[0] == '<' {
		return SpecFormatHTML
	}

	return SpecFormatUnknown
}

func looksLikeOpenAPIJSON(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, `"openapi"`) || strings.Contains(s, `"swagger"`)
}

func looksLikeOpenAPIYAML(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.HasPrefix(strings.TrimSpace(s), "openapi:") || strings.HasPrefix(strings.TrimSpace(s), "swagger:")
}

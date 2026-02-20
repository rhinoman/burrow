// Package configure provides conversational and structured configuration for Burrow.
package configure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/synthesis"
)

// DetectOllama checks if Ollama is running at localhost:11434 and returns
// a provider if available. Returns nil if Ollama is unreachable.
func DetectOllama() synthesis.Provider {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	// Parse response to find the best available model.
	// Prefers models with "instruct" in the name, otherwise picks the first.
	var tags struct {
		Models []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil || len(tags.Models) == 0 {
		return nil
	}

	best := tags.Models[0].Name
	bestSize := tags.Models[0].Size
	for _, m := range tags.Models[1:] {
		if m.Size > bestSize {
			best = m.Name
			bestSize = m.Size
		}
	}

	return synthesis.NewOllamaProvider("http://localhost:11434", best)
}

// DetectProvider scans configured LLM providers and returns the first usable one.
// Skips passthrough and empty types. Returns nil if no provider is available.
func DetectProvider(cfg *config.Config) synthesis.Provider {
	for _, p := range cfg.LLM.Providers {
		if p.Type == "" || p.Type == "passthrough" {
			continue
		}
		provider, err := synthesis.NewProvider(p)
		if err != nil || provider == nil {
			continue
		}
		return provider
	}
	return nil
}

// VerifyProvider checks that a provider can actually respond.
func VerifyProvider(ctx context.Context, provider synthesis.Provider) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := provider.Complete(ctx, "Respond with OK.", "Test")
	return err == nil
}

// FormatProviderStatus returns a human-readable description of provider detection results.
func FormatProviderStatus(provider synthesis.Provider) string {
	if provider == nil {
		return "No LLM detected"
	}
	return fmt.Sprintf("LLM available: %T", provider)
}

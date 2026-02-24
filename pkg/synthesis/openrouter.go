package synthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenRouterProvider implements Provider using the OpenAI-compatible chat completions API.
type OpenRouterProvider struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

// NewOpenRouterProvider creates a provider for OpenRouter or any OpenAI-compatible endpoint.
// Default endpoint is https://openrouter.ai/api/v1 if empty. Default timeout is 2 minutes.
func NewOpenRouterProvider(endpoint, apiKey, model string) *OpenRouterProvider {
	return NewOpenRouterProviderWithTimeout(endpoint, apiKey, model, 0)
}

// NewOpenRouterProviderWithTimeout creates an OpenRouter provider with a custom timeout.
// A timeout of 0 uses the default (2 minutes).
func NewOpenRouterProviderWithTimeout(endpoint, apiKey, model string, timeoutSecs int) *OpenRouterProvider {
	if endpoint == "" {
		endpoint = "https://openrouter.ai/api/v1"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	timeout := 2 * time.Minute
	if timeoutSecs > 0 {
		timeout = time.Duration(timeoutSecs) * time.Second
	}
	return &OpenRouterProvider{
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    model,
		client:   &http.Client{Timeout: timeout},
	}
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
	Error   *openAIError   `json:"error,omitempty"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

type openAIError struct {
	Message string `json:"message"`
}

// Complete sends a chat completion request using the OpenAI-compatible API.
func (o *OpenRouterProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []openAIMessage{
		{Role: "user", Content: userPrompt},
	}
	if systemPrompt != "" {
		messages = append([]openAIMessage{{Role: "system", Content: systemPrompt}}, messages...)
	}

	body, err := json.Marshal(openAIRequest{
		Model:    o.model,
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("invalid API key")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		// Try to extract error message from body
		var errResp openAIResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("API error HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result openAIResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

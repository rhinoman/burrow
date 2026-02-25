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

// OllamaProvider implements Provider using a local Ollama instance.
type OllamaProvider struct {
	endpoint      string
	model         string
	contextWindow int
	genParams     GenerationParams
	client        *http.Client
}

// NewOllamaProvider creates a provider that talks to Ollama's /api/chat endpoint.
// Default endpoint is http://localhost:11434 if empty. Default timeout is 5 minutes.
func NewOllamaProvider(endpoint, model string) *OllamaProvider {
	return NewOllamaProviderWithTimeout(endpoint, model, 0, 0)
}

// NewOllamaProviderWithTimeout creates an Ollama provider with a custom timeout.
// A timeout of 0 uses the default (5 minutes). A contextWindow of 0 omits num_ctx
// from requests, letting Ollama use its model default.
func NewOllamaProviderWithTimeout(endpoint, model string, timeoutSecs, contextWindow int) *OllamaProvider {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	timeout := 5 * time.Minute
	if timeoutSecs > 0 {
		timeout = time.Duration(timeoutSecs) * time.Second
	}
	return &OllamaProvider{
		endpoint:      endpoint,
		model:         model,
		contextWindow: contextWindow,
		client: &http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{},
		},
	}
}

// SetGenerationParams configures optional generation parameters.
func (o *OllamaProvider) SetGenerationParams(params GenerationParams) {
	o.genParams = params
}

type ollamaRequest struct {
	Model    string                 `json:"model"`
	Messages []ollamaMessage        `json:"messages"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Message ollamaMessage `json:"message"`
}

// Model returns the model name configured for this provider.
func (o *OllamaProvider) Model() string {
	return o.model
}

// Complete sends a chat completion request to Ollama.
func (o *OllamaProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []ollamaMessage{
		{Role: "user", Content: userPrompt},
	}
	if systemPrompt != "" {
		messages = append([]ollamaMessage{{Role: "system", Content: systemPrompt}}, messages...)
	}

	ollamaReq := ollamaRequest{
		Model:    o.model,
		Messages: messages,
		Stream:   false,
	}

	// Build options map with context window and generation params.
	opts := make(map[string]interface{})
	if o.contextWindow > 0 {
		opts["num_ctx"] = o.contextWindow
	}
	if o.genParams.Temperature != nil {
		opts["temperature"] = *o.genParams.Temperature
	}
	if o.genParams.TopP != nil {
		opts["top_p"] = *o.genParams.TopP
	}
	if o.genParams.MaxTokens > 0 {
		opts["num_predict"] = o.genParams.MaxTokens
	}
	if len(opts) > 0 {
		ollamaReq.Options = opts
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach Ollama at %s: %w", o.endpoint, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("model not found, run: ollama pull %s", o.model)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	return result.Message.Content, nil
}

package synthesis

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenRouterSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": [{"message": {"role": "assistant", "content": "# Report\nDone."}}]}`))
	}))
	defer srv.Close()

	p := NewOpenRouterProvider(srv.URL, "test-key", "mistral/mistral-7b")
	result, err := p.Complete(context.Background(), "Be brief.", "Generate report.")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result != "# Report\nDone." {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestOpenRouterAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "invalid key"}}`))
	}))
	defer srv.Close()

	p := NewOpenRouterProvider(srv.URL, "bad-key", "model")
	_, err := p.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "invalid API key") {
		t.Errorf("expected 'invalid API key' error, got: %s", err.Error())
	}
}

func TestOpenRouterRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": {"message": "too many requests"}}`))
	}))
	defer srv.Close()

	p := NewOpenRouterProvider(srv.URL, "key", "model")
	_, err := p.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected 'rate limited' error, got: %s", err.Error())
	}
}

func TestOpenRouterMalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": []}`))
	}))
	defer srv.Close()

	p := NewOpenRouterProvider(srv.URL, "key", "model")
	_, err := p.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("expected 'no choices' error, got: %s", err.Error())
	}
}

func TestOpenRouterGenerationParams(t *testing.T) {
	var capturedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": [{"message": {"role": "assistant", "content": "ok"}}]}`))
	}))
	defer srv.Close()

	temp := 0.3
	topP := 0.9
	p := NewOpenRouterProviderWithTimeout(srv.URL, "key", "model", 0)
	p.SetGenerationParams(GenerationParams{
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   4096,
	})

	_, err := p.Complete(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if v, ok := capturedBody["temperature"]; !ok || v.(float64) != 0.3 {
		t.Errorf("expected temperature=0.3, got %v", v)
	}
	if v, ok := capturedBody["top_p"]; !ok || v.(float64) != 0.9 {
		t.Errorf("expected top_p=0.9, got %v", v)
	}
	if v, ok := capturedBody["max_tokens"]; !ok || int(v.(float64)) != 4096 {
		t.Errorf("expected max_tokens=4096, got %v", v)
	}
}

func TestOpenRouterNoGenerationParams(t *testing.T) {
	var capturedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": [{"message": {"role": "assistant", "content": "ok"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenRouterProvider(srv.URL, "key", "model")
	_, err := p.Complete(context.Background(), "", "user")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// temperature, top_p, max_tokens should be absent (omitempty)
	if _, ok := capturedBody["temperature"]; ok {
		t.Error("expected temperature absent when not set")
	}
	if _, ok := capturedBody["top_p"]; ok {
		t.Error("expected top_p absent when not set")
	}
	if _, ok := capturedBody["max_tokens"]; ok {
		t.Error("expected max_tokens absent when not set")
	}
}

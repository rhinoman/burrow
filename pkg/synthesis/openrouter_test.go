package synthesis

import (
	"context"
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

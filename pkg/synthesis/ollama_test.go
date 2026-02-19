package synthesis

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": {"role": "assistant", "content": "# Report\nAnalysis complete."}}`))
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "qwen2.5:14b")
	result, err := p.Complete(context.Background(), "Be concise.", "Generate report.")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result != "# Report\nAnalysis complete." {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestOllamaModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "model not found"}`))
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "nonexistent")
	_, err := p.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if got := err.Error(); !strings.Contains(got, "model not found") || !strings.Contains(got, "ollama pull nonexistent") {
		t.Errorf("expected helpful error message, got: %s", got)
	}
}

func TestOllamaConnectionError(t *testing.T) {
	p := NewOllamaProvider("http://127.0.0.1:1", "test")
	_, err := p.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if got := err.Error(); !strings.Contains(got, "cannot reach Ollama") {
		t.Errorf("expected connection error message, got: %s", got)
	}
}

func TestOllamaContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond — let the context cancel
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	p := NewOllamaProvider(srv.URL, "test")
	_, err := p.Complete(ctx, "", "test")
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}


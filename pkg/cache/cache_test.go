package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jcadam/burrow/pkg/services"
)

type mockService struct {
	name      string
	response  []byte
	err       error
	callCount atomic.Int32
}

func (m *mockService) Name() string { return m.name }
func (m *mockService) Execute(_ context.Context, tool string, _ map[string]string) (*services.Result, error) {
	m.callCount.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	return &services.Result{
		Service:   m.name,
		Tool:      tool,
		Data:      m.response,
		Timestamp: time.Now().UTC(),
	}, nil
}

type errorResultService struct {
	name      string
	callCount atomic.Int32
}

func (e *errorResultService) Name() string { return e.name }
func (e *errorResultService) Execute(_ context.Context, tool string, _ map[string]string) (*services.Result, error) {
	e.callCount.Add(1)
	return &services.Result{
		Service:   e.name,
		Tool:      tool,
		Timestamp: time.Now().UTC(),
		Error:     "upstream timeout",
	}, nil
}

func TestCacheMiss(t *testing.T) {
	cacheDir := t.TempDir()
	inner := &mockService{name: "test-api", response: []byte(`{"data": "value"}`)}
	cached := NewCachedService(inner, cacheDir, 3600)

	result, err := cached.Execute(context.Background(), "search", map[string]string{"q": "test"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(result.Data) != `{"data": "value"}` {
		t.Errorf("unexpected data: %s", result.Data)
	}
	if inner.callCount.Load() != 1 {
		t.Errorf("expected inner called once, got %d", inner.callCount.Load())
	}
}

func TestCacheHit(t *testing.T) {
	cacheDir := t.TempDir()
	inner := &mockService{name: "test-api", response: []byte(`{"data": "value"}`)}
	cached := NewCachedService(inner, cacheDir, 3600)

	// First call — miss.
	cached.Execute(context.Background(), "search", map[string]string{"q": "test"})

	// Second call — hit.
	result, err := cached.Execute(context.Background(), "search", map[string]string{"q": "test"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(result.Data) != `{"data": "value"}` {
		t.Errorf("unexpected data: %s", result.Data)
	}
	if inner.callCount.Load() != 1 {
		t.Errorf("expected inner called once (cached), got %d", inner.callCount.Load())
	}
}

func TestCacheExpired(t *testing.T) {
	cacheDir := t.TempDir()
	inner := &mockService{name: "test-api", response: []byte(`{"data": "value"}`)}
	// 1-second TTL.
	cached := NewCachedService(inner, cacheDir, 1)

	// First call — miss.
	cached.Execute(context.Background(), "search", map[string]string{"q": "test"})

	// Wait for TTL to expire.
	time.Sleep(1100 * time.Millisecond)

	// Second call — expired, calls inner again.
	_, err := cached.Execute(context.Background(), "search", map[string]string{"q": "test"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if inner.callCount.Load() != 2 {
		t.Errorf("expected inner called twice (expired), got %d", inner.callCount.Load())
	}
}

func TestErrorNotCached(t *testing.T) {
	cacheDir := t.TempDir()
	inner := &errorResultService{name: "error-api"}
	cached := NewCachedService(inner, cacheDir, 3600)

	// First call — error result.
	result, _ := cached.Execute(context.Background(), "fetch", nil)
	if result.Error == "" {
		t.Fatal("expected error result")
	}

	// Second call — should call inner again (errors not cached).
	cached.Execute(context.Background(), "fetch", nil)
	if inner.callCount.Load() != 2 {
		t.Errorf("expected inner called twice (errors not cached), got %d", inner.callCount.Load())
	}
}

func TestCorruptedCacheFile(t *testing.T) {
	cacheDir := t.TempDir()
	inner := &mockService{name: "test-api", response: []byte(`{"data": "value"}`)}
	cached := NewCachedService(inner, cacheDir, 3600)

	// First call — populates cache.
	cached.Execute(context.Background(), "search", map[string]string{"q": "test"})

	// Corrupt the cache file.
	dir := filepath.Join(cacheDir, "test-api")
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 cache file, got %d", len(entries))
	}
	os.WriteFile(filepath.Join(dir, entries[0].Name()), []byte("not json"), 0o644)

	// Second call — corrupted file treated as miss, calls inner.
	result, err := cached.Execute(context.Background(), "search", map[string]string{"q": "test"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(result.Data) != `{"data": "value"}` {
		t.Errorf("unexpected data: %s", result.Data)
	}
	if inner.callCount.Load() != 2 {
		t.Errorf("expected inner called twice (corrupted cache), got %d", inner.callCount.Load())
	}
}

func TestCacheFileIsValidJSON(t *testing.T) {
	cacheDir := t.TempDir()
	inner := &mockService{name: "test-api", response: []byte(`{"data": "value"}`)}
	cached := NewCachedService(inner, cacheDir, 3600)

	cached.Execute(context.Background(), "search", map[string]string{"q": "test"})

	// Read the cache file and verify it's valid JSON.
	dir := filepath.Join(cacheDir, "test-api")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 cache file, got %d", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("cache file is not valid JSON: %v", err)
	}
	if entry.Service != "test-api" {
		t.Errorf("expected service test-api, got %q", entry.Service)
	}
	if entry.Tool != "search" {
		t.Errorf("expected tool search, got %q", entry.Tool)
	}
	if !strings.HasSuffix(entries[0].Name(), ".json") {
		t.Errorf("expected .json extension, got %q", entries[0].Name())
	}
}

func TestCacheDifferentParams(t *testing.T) {
	cacheDir := t.TempDir()
	inner := &mockService{name: "test-api", response: []byte(`{"data": "value"}`)}
	cached := NewCachedService(inner, cacheDir, 3600)

	// Two calls with different params should both hit the inner service.
	cached.Execute(context.Background(), "search", map[string]string{"q": "alpha"})
	cached.Execute(context.Background(), "search", map[string]string{"q": "beta"})
	if inner.callCount.Load() != 2 {
		t.Errorf("expected inner called twice (different params), got %d", inner.callCount.Load())
	}

	// Re-calling with same params should hit cache.
	cached.Execute(context.Background(), "search", map[string]string{"q": "alpha"})
	if inner.callCount.Load() != 2 {
		t.Errorf("expected inner still at 2 (cached), got %d", inner.callCount.Load())
	}
}

func TestCacheName(t *testing.T) {
	inner := &mockService{name: "my-api"}
	cached := NewCachedService(inner, t.TempDir(), 3600)
	if cached.Name() != "my-api" {
		t.Errorf("expected name my-api, got %q", cached.Name())
	}
}

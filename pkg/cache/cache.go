// Package cache provides a file-based result caching decorator for services.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jcadam/burrow/pkg/services"
)

// CachedService wraps a Service with file-based result caching.
type CachedService struct {
	inner    services.Service
	cacheDir string
	ttl      time.Duration
}

// NewCachedService wraps a service with TTL-based file caching.
// Cache files are stored under cacheDir/<service-name>/.
func NewCachedService(inner services.Service, cacheDir string, ttlSeconds int) *CachedService {
	return &CachedService{
		inner:    inner,
		cacheDir: cacheDir,
		ttl:      time.Duration(ttlSeconds) * time.Second,
	}
}

func (c *CachedService) Name() string { return c.inner.Name() }

// Execute checks the cache first, returning a cached result if valid.
// On miss or expiry, calls the inner service and caches successful results.
func (c *CachedService) Execute(ctx context.Context, tool string, params map[string]string) (*services.Result, error) {
	key := cacheKey(c.inner.Name(), tool, params)
	dir := filepath.Join(c.cacheDir, c.inner.Name())

	// Try cache hit.
	if result, ok := c.readCache(dir, key); ok {
		return result, nil
	}

	// Cache miss — call inner service.
	result, err := c.inner.Execute(ctx, tool, params)
	if err != nil {
		return result, err
	}

	// Don't cache error results (transient failures shouldn't persist).
	if result.Error == "" {
		c.writeCache(dir, key, tool, params, result)
	}

	return result, nil
}

// cacheEntry is the JSON format stored on disk (inspectable with cat).
type cacheEntry struct {
	Service    string            `json:"service"`
	Tool       string            `json:"tool"`
	Params     map[string]string `json:"params"`
	Timestamp  time.Time         `json:"timestamp"`
	TTLSeconds int               `json:"ttl_seconds"`
	Data       string            `json:"data"` // base64-encoded
	Error      string            `json:"error"`
}

func cacheKey(service, tool string, params map[string]string) string {
	// Sort params for deterministic key.
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}

	input := service + "\x00" + tool + "\x00" + strings.Join(parts, "\x00")
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:16]) // 32 hex chars — collision-free for practical use
}

func cacheFilePath(dir, key string) string {
	return filepath.Join(dir, key+".json")
}

func (c *CachedService) readCache(dir, key string) (*services.Result, bool) {
	path := cacheFilePath(dir, key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Corrupted cache file — delete and treat as miss.
		os.Remove(path)
		return nil, false
	}

	// Check TTL.
	if time.Since(entry.Timestamp) > c.ttl {
		return nil, false
	}

	decoded, err := base64.StdEncoding.DecodeString(entry.Data)
	if err != nil {
		os.Remove(path)
		return nil, false
	}

	return &services.Result{
		Service:   entry.Service,
		Tool:      entry.Tool,
		Data:      decoded,
		Timestamp: entry.Timestamp,
		Error:     entry.Error,
	}, true
}

func (c *CachedService) writeCache(dir, key, tool string, params map[string]string, result *services.Result) {
	// Lazy directory creation.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return // best-effort
	}

	entry := cacheEntry{
		Service:    c.inner.Name(),
		Tool:       tool,
		Params:     params,
		Timestamp:  result.Timestamp,
		TTLSeconds: int(c.ttl.Seconds()),
		Data:       base64.StdEncoding.EncodeToString(result.Data),
		Error:      result.Error,
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(cacheFilePath(dir, key), data, 0o644)
}

// Package services defines the service interface and registry.
package services

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Service is the interface all service adapters must implement.
type Service interface {
	Name() string
	Execute(ctx context.Context, tool string, params map[string]string) (*Result, error)
}

// Result holds the output of a single service query.
type Result struct {
	Service      string
	Tool         string
	Data         []byte
	URL          string // the request URL (for debugging)
	Timestamp    time.Time
	Error        string
	ContextLabel string // user-provided label for better synthesis prompts (e.g., "NWS 7-Day Forecast — Anchorage")
}

// Registry manages named service instances.
type Registry struct {
	mu       sync.RWMutex
	services map[string]Service
}

// NewRegistry creates an empty service registry.
func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]Service),
	}
}

// Register adds a service to the registry. Returns an error if a service
// with the same name is already registered.
func (r *Registry) Register(svc Service) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := svc.Name()
	if _, exists := r.services[name]; exists {
		return fmt.Errorf("service already registered: %q", name)
	}
	r.services[name] = svc
	return nil
}

// Get retrieves a service by name.
func (r *Registry) Get(name string) (Service, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	svc, ok := r.services[name]
	if !ok {
		return nil, fmt.Errorf("service not found: %q", name)
	}
	return svc, nil
}

// List returns the names of all registered services, sorted alphabetically.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.services))
	for name := range r.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

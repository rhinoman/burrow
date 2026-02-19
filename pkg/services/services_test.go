package services

import (
	"context"
	"testing"
)

type fakeService struct {
	name string
}

func (f *fakeService) Name() string { return f.name }
func (f *fakeService) Execute(ctx context.Context, tool string, params map[string]string) (*Result, error) {
	return &Result{Service: f.name, Tool: tool, Data: []byte("test")}, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	svc := &fakeService{name: "test-svc"}

	if err := r.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Get("test-svc")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "test-svc" {
		t.Errorf("expected test-svc, got %q", got.Name())
	}
}

func TestRegistryDuplicate(t *testing.T) {
	r := NewRegistry()
	svc := &fakeService{name: "dup"}

	if err := r.Register(svc); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register(svc); err == nil {
		t.Fatal("expected error registering duplicate")
	}
}

func TestRegistryNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("missing")
	if err == nil {
		t.Fatal("expected error for missing service")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeService{name: "alpha"})
	r.Register(&fakeService{name: "beta"})

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["alpha"] || !nameSet["beta"] {
		t.Errorf("expected alpha and beta, got %v", names)
	}
}

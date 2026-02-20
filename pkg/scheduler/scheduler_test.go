package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jcadam/burrow/pkg/pipeline"
)

// --- Test Clock ---

type testClock struct {
	mu  sync.Mutex
	now time.Time
	ch  chan time.Time
}

func newTestClock(now time.Time) *testClock {
	return &testClock{
		now: now,
		ch:  make(chan time.Time, 10),
	}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Tick(time.Duration) <-chan time.Time {
	return c.ch
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	c.mu.Unlock()
	c.ch <- now
}

// --- parseSchedule tests ---

func TestParseSchedule(t *testing.T) {
	tests := []struct {
		input   string
		hour    int
		minute  int
		wantErr bool
	}{
		{"05:00", 5, 0, false},
		{"23:59", 23, 59, false},
		{"00:00", 0, 0, false},
		{"5:00", 5, 0, false},     // single digit hour
		{"'05:00'", 5, 0, false},  // YAML single quotes
		{"\"05:00\"", 5, 0, false}, // YAML double quotes
		{"12:30", 12, 30, false},
		{"25:00", 0, 0, true},  // hour out of range
		{"12:60", 0, 0, true},  // minute out of range
		{"-1:00", 0, 0, true},  // negative
		{"abc", 0, 0, true},    // not a time
		{"", 0, 0, true},       // empty
		{"12", 0, 0, true},     // no colon
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			h, m, err := parseSchedule(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got hour=%d minute=%d", tt.input, h, m)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if h != tt.hour || m != tt.minute {
				t.Errorf("parseSchedule(%q) = %d:%d, want %d:%d", tt.input, h, m, tt.hour, tt.minute)
			}
		})
	}
}

// --- isDue tests ---

func TestIsDue(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name        string
		now         time.Time
		schedule    string
		lastRunDate string
		want        bool
	}{
		{
			name:     "not yet — before schedule time",
			now:      time.Date(2025, 1, 15, 4, 59, 0, 0, loc),
			schedule: "05:00",
			want:     false,
		},
		{
			name:     "exact — schedule time",
			now:      time.Date(2025, 1, 15, 5, 0, 0, 0, loc),
			schedule: "05:00",
			want:     true,
		},
		{
			name:     "just passed",
			now:      time.Date(2025, 1, 15, 5, 1, 0, 0, loc),
			schedule: "05:00",
			want:     true,
		},
		{
			name:     "late daemon start",
			now:      time.Date(2025, 1, 15, 23, 0, 0, 0, loc),
			schedule: "05:00",
			want:     true,
		},
		{
			name:        "already ran today",
			now:         time.Date(2025, 1, 15, 5, 1, 0, 0, loc),
			schedule:    "05:00",
			lastRunDate: "2025-01-15",
			want:        false,
		},
		{
			name:        "ran yesterday — due today",
			now:         time.Date(2025, 1, 15, 5, 1, 0, 0, loc),
			schedule:    "05:00",
			lastRunDate: "2025-01-14",
			want:        true,
		},
		{
			name:        "first run — no state",
			now:         time.Date(2025, 1, 15, 5, 1, 0, 0, loc),
			schedule:    "05:00",
			lastRunDate: "",
			want:        true,
		},
		{
			name:     "invalid schedule",
			now:      time.Date(2025, 1, 15, 5, 1, 0, 0, loc),
			schedule: "bad",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDue(tt.now, tt.schedule, loc, tt.lastRunDate)
			if got != tt.want {
				t.Errorf("isDue(%v, %q, UTC, %q) = %v, want %v",
					tt.now.Format("15:04"), tt.schedule, tt.lastRunDate, got, tt.want)
			}
		})
	}
}

func TestIsDueTimezone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}

	// 09:00 UTC on Jan 15 = 04:00 NY on Jan 15 (winter, UTC-5).
	// Schedule is 05:00 NY. 04:00 < 05:00 → NOT due.
	nowUTC := time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC)
	got := isDue(nowUTC, "05:00", ny, "")
	if got {
		t.Error("should not be due: 09:00 UTC = 04:00 NY, before 05:00 NY schedule")
	}

	// 11:00 UTC on Jan 15 = 06:00 NY on Jan 15 → past 05:00 → due.
	nowUTC2 := time.Date(2025, 1, 15, 11, 0, 0, 0, time.UTC)
	got2 := isDue(nowUTC2, "05:00", ny, "")
	if !got2 {
		t.Error("should be due: 11:00 UTC = 06:00 NY, past 05:00 NY schedule")
	}

	// Same time but already ran today (in NY timezone).
	today := nowUTC2.In(ny).Format("2006-01-02")
	got3 := isDue(nowUTC2, "05:00", ny, today)
	if got3 {
		t.Error("should not be due: already ran today in NY timezone")
	}

	// "today" in NY is different from "today" in UTC near midnight.
	// 04:30 UTC on Jan 15 = 23:30 NY on Jan 14. Schedule 23:00 NY.
	// 23:30 > 23:00 on Jan 14 → due for Jan 14.
	nowUTC3 := time.Date(2025, 1, 15, 4, 30, 0, 0, time.UTC)
	got4 := isDue(nowUTC3, "23:00", ny, "")
	if !got4 {
		t.Error("should be due: 04:30 UTC = 23:30 NY Jan 14, past 23:00 NY schedule")
	}

	// But if already ran on Jan 14 → not due.
	got5 := isDue(nowUTC3, "23:00", ny, "2025-01-14")
	if got5 {
		t.Error("should not be due: already ran on 2025-01-14 in NY timezone")
	}
}

// --- Scheduler integration tests ---

func TestSchedulerRunsRoutineWhenDue(t *testing.T) {
	clock := newTestClock(time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC))
	store := NewMemoryStateStore()
	var ran atomic.Int32

	routine := &pipeline.Routine{
		Name:     "morning-brief",
		Schedule: "05:00",
		Timezone: "UTC",
	}

	s := New(Config{
		Clock:  clock,
		Store:  store,
		Loader: func() ([]*pipeline.Routine, error) { return []*pipeline.Routine{routine}, nil },
		Runner: func(ctx context.Context, r *pipeline.Routine) error {
			ran.Add(1)
			return nil
		},
		Once: true,
	})

	err := s.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if ran.Load() != 1 {
		t.Errorf("runner called %d times, want 1", ran.Load())
	}

	// Verify state was updated.
	state, _ := store.Load()
	if state.LastRun["morning-brief"] != "2025-01-15" {
		t.Errorf("last run = %q, want %q", state.LastRun["morning-brief"], "2025-01-15")
	}
}

func TestSchedulerSkipsAlreadyRunToday(t *testing.T) {
	clock := newTestClock(time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC))
	store := NewMemoryStateStore()
	store.Save(&State{LastRun: map[string]string{"morning-brief": "2025-01-15"}})
	var ran atomic.Int32

	routine := &pipeline.Routine{
		Name:     "morning-brief",
		Schedule: "05:00",
		Timezone: "UTC",
	}

	s := New(Config{
		Clock:  clock,
		Store:  store,
		Loader: func() ([]*pipeline.Routine, error) { return []*pipeline.Routine{routine}, nil },
		Runner: func(ctx context.Context, r *pipeline.Routine) error {
			ran.Add(1)
			return nil
		},
		Once: true,
	})

	s.Run(context.Background())

	if ran.Load() != 0 {
		t.Errorf("runner called %d times, want 0 (already ran today)", ran.Load())
	}
}

func TestSchedulerSkipsNoSchedule(t *testing.T) {
	clock := newTestClock(time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC))
	store := NewMemoryStateStore()
	var ran atomic.Int32

	routine := &pipeline.Routine{
		Name: "manual-only",
		// No Schedule field
	}

	s := New(Config{
		Clock:  clock,
		Store:  store,
		Loader: func() ([]*pipeline.Routine, error) { return []*pipeline.Routine{routine}, nil },
		Runner: func(ctx context.Context, r *pipeline.Routine) error {
			ran.Add(1)
			return nil
		},
		Once: true,
	})

	s.Run(context.Background())

	if ran.Load() != 0 {
		t.Errorf("runner called %d times, want 0 (no schedule)", ran.Load())
	}
}

func TestSchedulerSkipsInflight(t *testing.T) {
	clock := newTestClock(time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC))
	store := NewMemoryStateStore()

	started := make(chan struct{})
	proceed := make(chan struct{})
	var runCount atomic.Int32

	routine := &pipeline.Routine{
		Name:     "slow-routine",
		Schedule: "05:00",
		Timezone: "UTC",
	}

	s := New(Config{
		Clock: clock,
		Store: store,
		Loader: func() ([]*pipeline.Routine, error) {
			return []*pipeline.Routine{routine}, nil
		},
		Runner: func(ctx context.Context, r *pipeline.Routine) error {
			runCount.Add(1)
			started <- struct{}{}
			<-proceed
			return nil
		},
		Once: false,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	// Wait for first run to start.
	<-started

	// Trigger another tick while the first is still running.
	// Reset state so isDue would return true if not for inflight check.
	store.Save(&State{LastRun: make(map[string]string)})
	clock.Advance(1 * time.Minute)

	// Give the tick time to evaluate.
	time.Sleep(50 * time.Millisecond)

	// Should still be 1 — second tick skipped because inflight.
	if runCount.Load() != 1 {
		t.Errorf("run count = %d, want 1 (second run should be skipped while inflight)", runCount.Load())
	}

	close(proceed)
	cancel()
}

func TestSchedulerOnceMode(t *testing.T) {
	clock := newTestClock(time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC))
	store := NewMemoryStateStore()
	var ran atomic.Int32

	routine := &pipeline.Routine{
		Name:     "once-test",
		Schedule: "05:00",
		Timezone: "UTC",
	}

	s := New(Config{
		Clock:  clock,
		Store:  store,
		Loader: func() ([]*pipeline.Routine, error) { return []*pipeline.Routine{routine}, nil },
		Runner: func(ctx context.Context, r *pipeline.Routine) error {
			ran.Add(1)
			return nil
		},
		Once: true,
	})

	// Run should return after a single evaluation.
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return in once mode")
	}

	if ran.Load() != 1 {
		t.Errorf("runner called %d times, want 1", ran.Load())
	}
}

func TestSchedulerReloadsRoutines(t *testing.T) {
	clock := newTestClock(time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC))
	store := NewMemoryStateStore()

	var mu sync.Mutex
	routines := []*pipeline.Routine{
		{Name: "first", Schedule: "05:00", Timezone: "UTC"},
	}
	var names []string

	s := New(Config{
		Clock: clock,
		Store: store,
		Loader: func() ([]*pipeline.Routine, error) {
			mu.Lock()
			defer mu.Unlock()
			cp := make([]*pipeline.Routine, len(routines))
			copy(cp, routines)
			return cp, nil
		},
		Runner: func(ctx context.Context, r *pipeline.Routine) error {
			mu.Lock()
			names = append(names, r.Name)
			mu.Unlock()
			return nil
		},
		Once: false,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	// Wait for first tick to complete.
	time.Sleep(100 * time.Millisecond)

	// Add a new routine and advance clock to next day so both are due.
	mu.Lock()
	routines = append(routines, &pipeline.Routine{Name: "second", Schedule: "05:00", Timezone: "UTC"})
	mu.Unlock()

	// Set clock to next day so both routines are due again.
	clock.mu.Lock()
	clock.now = time.Date(2025, 1, 16, 5, 1, 0, 0, time.UTC)
	clock.mu.Unlock()
	clock.ch <- clock.Now()

	time.Sleep(100 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()

	hasFirst := false
	hasSecond := false
	for _, n := range names {
		if n == "first" {
			hasFirst = true
		}
		if n == "second" {
			hasSecond = true
		}
	}

	if !hasFirst {
		t.Error("first routine never ran")
	}
	if !hasSecond {
		t.Error("second routine (added after start) never ran")
	}
}

func TestSchedulerFailedRunRetries(t *testing.T) {
	clock := newTestClock(time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC))
	store := NewMemoryStateStore()

	var callCount atomic.Int32

	routine := &pipeline.Routine{
		Name:     "flaky",
		Schedule: "05:00",
		Timezone: "UTC",
	}

	s := New(Config{
		Clock: clock,
		Store: store,
		Loader: func() ([]*pipeline.Routine, error) {
			return []*pipeline.Routine{routine}, nil
		},
		Runner: func(ctx context.Context, r *pipeline.Routine) error {
			n := callCount.Add(1)
			if n == 1 {
				return fmt.Errorf("temporary failure")
			}
			return nil
		},
		Once: false,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	// Wait for first tick (will fail).
	time.Sleep(100 * time.Millisecond)

	// State should NOT have been updated.
	state, _ := store.Load()
	if _, ok := state.LastRun["flaky"]; ok {
		t.Fatal("state should not be updated after failed run")
	}

	// Advance clock — same day, routine should retry.
	clock.Advance(1 * time.Minute)
	time.Sleep(100 * time.Millisecond)

	cancel()

	if callCount.Load() < 2 {
		t.Errorf("runner called %d times, want >= 2 (should retry after failure)", callCount.Load())
	}

	// State should now be updated (second run succeeded).
	state, _ = store.Load()
	if state.LastRun["flaky"] != "2025-01-15" {
		t.Errorf("last run = %q, want %q", state.LastRun["flaky"], "2025-01-15")
	}
}

func TestSchedulerConcurrentCompletionsBothPersist(t *testing.T) {
	clock := newTestClock(time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC))
	store := NewMemoryStateStore()

	// Two routines that finish near-simultaneously.
	gate := make(chan struct{})

	routines := []*pipeline.Routine{
		{Name: "alpha", Schedule: "05:00", Timezone: "UTC"},
		{Name: "beta", Schedule: "05:00", Timezone: "UTC"},
	}

	s := New(Config{
		Clock: clock,
		Store: store,
		Loader: func() ([]*pipeline.Routine, error) {
			return routines, nil
		},
		Runner: func(ctx context.Context, r *pipeline.Routine) error {
			// Both goroutines block until gate is closed, then finish together.
			<-gate
			return nil
		},
		Once: true,
	})

	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()

	// Give both goroutines time to start and block on gate.
	time.Sleep(50 * time.Millisecond)

	// Release both at the same time — they'll race to update state.
	close(gate)

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return")
	}

	// Both routines' last_run should be recorded.
	state, _ := store.Load()
	if state.LastRun["alpha"] != "2025-01-15" {
		t.Errorf("alpha last run = %q, want %q", state.LastRun["alpha"], "2025-01-15")
	}
	if state.LastRun["beta"] != "2025-01-15" {
		t.Errorf("beta last run = %q, want %q", state.LastRun["beta"], "2025-01-15")
	}
}

func TestSchedulerLogsInvalidSchedule(t *testing.T) {
	clock := newTestClock(time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC))
	store := NewMemoryStateStore()
	var buf strings.Builder
	var ran atomic.Int32

	routine := &pipeline.Routine{
		Name:     "bad-schedule",
		Schedule: "every morning",
		Timezone: "UTC",
	}

	s := New(Config{
		Clock:  clock,
		Store:  store,
		Logger: &buf,
		Loader: func() ([]*pipeline.Routine, error) { return []*pipeline.Routine{routine}, nil },
		Runner: func(ctx context.Context, r *pipeline.Routine) error {
			ran.Add(1)
			return nil
		},
		Once: true,
	})

	s.Run(context.Background())

	if ran.Load() != 0 {
		t.Error("runner should not have been called for invalid schedule")
	}
	if !strings.Contains(buf.String(), "invalid schedule") {
		t.Errorf("expected log about invalid schedule, got: %q", buf.String())
	}
}

// --- FileStateStore tests ---

func TestFileStateStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := NewFileStateStore(path)

	original := &State{
		LastRun: map[string]string{
			"morning-brief": "2025-01-15",
			"evening-scan":  "2025-01-14",
		},
	}

	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.LastRun) != 2 {
		t.Fatalf("loaded %d entries, want 2", len(loaded.LastRun))
	}
	if loaded.LastRun["morning-brief"] != "2025-01-15" {
		t.Errorf("morning-brief = %q, want %q", loaded.LastRun["morning-brief"], "2025-01-15")
	}
	if loaded.LastRun["evening-scan"] != "2025-01-14" {
		t.Errorf("evening-scan = %q, want %q", loaded.LastRun["evening-scan"], "2025-01-14")
	}
}

func TestFileStateStoreMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	store := NewFileStateStore(path)

	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.LastRun) != 0 {
		t.Errorf("expected empty state, got %d entries", len(state.LastRun))
	}
}

func TestFileStateStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := NewFileStateStore(path)

	if err := store.Save(&State{LastRun: map[string]string{"test": "2025-01-15"}}); err != nil {
		t.Fatal(err)
	}

	// Verify the file exists and is valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("state file is not valid JSON: %v", err)
	}

	// Verify no temp files left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestFileStateStoreCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "state.json")
	store := NewFileStateStore(path)

	if err := store.Save(&State{LastRun: map[string]string{"test": "2025-01-15"}}); err != nil {
		t.Fatal(err)
	}

	// Verify it can be loaded back.
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.LastRun["test"] != "2025-01-15" {
		t.Errorf("test = %q, want %q", state.LastRun["test"], "2025-01-15")
	}
}

// --- routineLocation tests ---

func TestRoutineLocation(t *testing.T) {
	r := &pipeline.Routine{Timezone: "America/New_York"}
	loc, err := routineLocation(r)
	if err != nil {
		t.Fatal(err)
	}
	if loc.String() != "America/New_York" {
		t.Errorf("location = %q, want %q", loc.String(), "America/New_York")
	}

	// Empty timezone falls back to local.
	r2 := &pipeline.Routine{}
	loc2, err := routineLocation(r2)
	if err != nil {
		t.Fatal(err)
	}
	if loc2 != time.Local {
		t.Errorf("expected time.Local for empty timezone, got %q", loc2.String())
	}

	// Invalid timezone.
	r3 := &pipeline.Routine{Timezone: "Not/A/Timezone"}
	_, err = routineLocation(r3)
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
}

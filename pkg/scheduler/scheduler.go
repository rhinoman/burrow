// Package scheduler implements time-based routine scheduling for Burrow.
// It ticks every minute, checks which routines are due based on their
// Schedule/Timezone fields, and executes them via a caller-provided runner.
// The scheduler never listens on a port or accepts inbound connections.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // embedded timezone database for minimal systems

	"github.com/jcadam/burrow/pkg/pipeline"
)

// Clock abstracts time for testability.
type Clock interface {
	Now() time.Time
	Tick(d time.Duration) <-chan time.Time
}

// SystemClock uses the real system clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time                         { return time.Now() }
func (SystemClock) Tick(d time.Duration) <-chan time.Time   { return time.Tick(d) }

// RoutineRunner executes a single routine. Provided by the caller (cmd/gd).
type RoutineRunner func(ctx context.Context, routine *pipeline.Routine) error

// RoutineLoader loads all current routines. Called each tick.
type RoutineLoader func() ([]*pipeline.Routine, error)

// State tracks last-run date (YYYY-MM-DD in routine's timezone) per routine name.
type State struct {
	LastRun map[string]string `json:"last_run"`
}

// StateStore abstracts state persistence.
type StateStore interface {
	Load() (*State, error)
	Save(s *State) error
}

// Config holds all dependencies for the scheduler.
type Config struct {
	Clock  Clock          // defaults to SystemClock
	Store  StateStore     // state persistence
	Loader RoutineLoader  // routine loading
	Runner RoutineRunner  // routine execution
	Logger io.Writer      // log output (os.Stderr in prod)
	Once   bool           // single evaluation pass, then exit
}

// Scheduler evaluates routine schedules and launches executions.
type Scheduler struct {
	cfg      Config
	inflight map[string]bool
	mu       sync.Mutex    // guards inflight map
	stateMu  sync.Mutex    // serializes state load→modify→save
	wg       sync.WaitGroup
}

// New creates a scheduler with the given config.
func New(cfg Config) *Scheduler {
	if cfg.Clock == nil {
		cfg.Clock = SystemClock{}
	}
	if cfg.Logger == nil {
		cfg.Logger = io.Discard
	}
	return &Scheduler{
		cfg:      cfg,
		inflight: make(map[string]bool),
	}
}

// Run blocks until ctx is cancelled. Ticks every minute.
// If Config.Once, performs one evaluation pass and returns.
func (s *Scheduler) Run(ctx context.Context) error {
	// Run one tick immediately on start.
	s.tick(ctx)

	if s.cfg.Once {
		s.wg.Wait()
		return nil
	}

	ch := s.cfg.Clock.Tick(1 * time.Minute)
	for {
		select {
		case <-ctx.Done():
			s.wg.Wait()
			return ctx.Err()
		case _, ok := <-ch:
			if !ok {
				s.wg.Wait()
				return nil
			}
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	routines, err := s.cfg.Loader()
	if err != nil {
		fmt.Fprintf(s.cfg.Logger, "error loading routines: %v\n", err)
		return
	}

	state, err := s.cfg.Store.Load()
	if err != nil {
		fmt.Fprintf(s.cfg.Logger, "error loading state: %v\n", err)
		return
	}

	now := s.cfg.Clock.Now()

	for _, routine := range routines {
		if routine.Schedule == "" {
			continue
		}

		if _, _, err := parseSchedule(routine.Schedule); err != nil {
			fmt.Fprintf(s.cfg.Logger, "routine %q: invalid schedule %q: %v\n", routine.Name, routine.Schedule, err)
			continue
		}

		loc, err := routineLocation(routine)
		if err != nil {
			fmt.Fprintf(s.cfg.Logger, "routine %q: bad timezone %q: %v\n", routine.Name, routine.Timezone, err)
			continue
		}

		lastRun := state.LastRun[routine.Name]
		if !isDue(now, routine.Schedule, loc, lastRun) {
			continue
		}

		s.mu.Lock()
		if s.inflight[routine.Name] {
			s.mu.Unlock()
			continue
		}
		s.inflight[routine.Name] = true
		s.mu.Unlock()

		r := routine // capture for goroutine
		today := now.In(loc).Format("2006-01-02")

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				delete(s.inflight, r.Name)
				s.mu.Unlock()
			}()

			fmt.Fprintf(s.cfg.Logger, "running routine %q (schedule %s)\n", r.Name, r.Schedule)
			if err := s.cfg.Runner(ctx, r); err != nil {
				fmt.Fprintf(s.cfg.Logger, "routine %q failed: %v\n", r.Name, err)
				return // don't record failed runs — will retry next tick
			}

			fmt.Fprintf(s.cfg.Logger, "routine %q completed\n", r.Name)

			// Record success. Mutex serializes concurrent load→modify→save
			// sequences to prevent one goroutine from clobbering another's write.
			s.stateMu.Lock()
			current, err := s.cfg.Store.Load()
			if err != nil {
				s.stateMu.Unlock()
				fmt.Fprintf(s.cfg.Logger, "error reloading state after %q: %v\n", r.Name, err)
				return
			}
			current.LastRun[r.Name] = today
			if err := s.cfg.Store.Save(current); err != nil {
				fmt.Fprintf(s.cfg.Logger, "error saving state after %q: %v\n", r.Name, err)
			}
			s.stateMu.Unlock()
		}()
	}
}

// parseSchedule parses "HH:MM" into hour and minute. Strips surrounding quotes
// that YAML may preserve.
func parseSchedule(s string) (int, int, error) {
	s = strings.Trim(s, "'\"")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, fmt.Errorf("empty schedule")
	}

	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid schedule format %q: expected HH:MM", s)
	}

	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid hour in %q: %w", s, err)
	}
	minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minute in %q: %w", s, err)
	}

	if hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("hour %d out of range 0-23", hour)
	}
	if minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("minute %d out of range 0-59", minute)
	}

	return hour, minute, nil
}

// isDue returns true if the schedule time has passed today (in loc) and the
// routine has not yet been run today. lastRunDate is "YYYY-MM-DD" or empty.
func isDue(now time.Time, schedule string, loc *time.Location, lastRunDate string) bool {
	hour, minute, err := parseSchedule(schedule)
	if err != nil {
		return false
	}

	nowLocal := now.In(loc)
	today := nowLocal.Format("2006-01-02")

	// Already ran today.
	if lastRunDate == today {
		return false
	}

	// Schedule time hasn't arrived yet today.
	scheduleTime := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), hour, minute, 0, 0, loc)
	if nowLocal.Before(scheduleTime) {
		return false
	}

	return true
}

// routineLocation returns the time.Location for a routine's Timezone field.
// Falls back to time.Local if empty.
func routineLocation(r *pipeline.Routine) (*time.Location, error) {
	if r.Timezone == "" {
		return time.Local, nil
	}
	return time.LoadLocation(r.Timezone)
}

// --- FileStateStore ---

// FileStateStore persists scheduler state to a JSON file.
type FileStateStore struct {
	path string
}

// NewFileStateStore creates a FileStateStore at the given path.
func NewFileStateStore(path string) *FileStateStore {
	return &FileStateStore{path: path}
}

// Load reads the state file. Returns empty state if the file doesn't exist.
func (f *FileStateStore) Load() (*State, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{LastRun: make(map[string]string)}, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	if s.LastRun == nil {
		s.LastRun = make(map[string]string)
	}
	return &s, nil
}

// Save writes the state file atomically via temp+rename.
func (f *FileStateStore) Save(s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "scheduler-state-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, f.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming state file: %w", err)
	}
	return nil
}

// --- MemoryStateStore ---

// MemoryStateStore is an in-memory StateStore for testing.
type MemoryStateStore struct {
	mu    sync.Mutex
	state *State
}

// NewMemoryStateStore creates an empty in-memory state store.
func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		state: &State{LastRun: make(map[string]string)},
	}
}

// Load returns a copy of the current state.
func (m *MemoryStateStore) Load() (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := &State{LastRun: make(map[string]string)}
	for k, v := range m.state.LastRun {
		cp.LastRun[k] = v
	}
	return cp, nil
}

// Save replaces the stored state with a copy.
func (m *MemoryStateStore) Save(s *State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := &State{LastRun: make(map[string]string)}
	for k, v := range s.LastRun {
		cp.LastRun[k] = v
	}
	m.state = cp
	return nil
}

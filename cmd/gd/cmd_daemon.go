package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/jcadam/burrow/pkg/config"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/jcadam/burrow/pkg/pipeline"
	"github.com/jcadam/burrow/pkg/scheduler"
	"github.com/spf13/cobra"
)

var daemonOnce bool

func init() {
	daemonCmd.Flags().BoolVar(&daemonOnce, "once", false, "Evaluate schedules once and exit (for cron integration)")
	rootCmd.AddCommand(daemonCmd)
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the routine scheduler",
	Long: `Runs the scheduler in the foreground. Evaluates routine schedules
every minute and executes due routines. Use --once for cron integration.
Send SIGINT or SIGTERM to stop gracefully.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}

		routinesDir := filepath.Join(burrowDir, "routines")
		statePath := filepath.Join(burrowDir, "scheduler-state.json")

		store := scheduler.NewFileStateStore(statePath)
		loader := func() ([]*pipeline.Routine, error) {
			return pipeline.LoadAllRoutines(routinesDir, os.Stderr)
		}
		runner := func(ctx context.Context, routine *pipeline.Routine) error {
			return runRoutine(ctx, burrowDir, routine)
		}

		sched := scheduler.New(scheduler.Config{
			Store:  store,
			Loader: loader,
			Runner: runner,
			Logger: os.Stderr,
			Once:   daemonOnce,
		})

		// Print startup banner.
		routines, _ := loader()
		scheduled := 0
		for _, r := range routines {
			if r.Schedule != "" {
				scheduled++
			}
		}
		if daemonOnce {
			fmt.Fprintf(os.Stderr, "Burrow scheduler: evaluating %d routine(s) with schedules (once mode)\n", scheduled)
		} else {
			fmt.Fprintf(os.Stderr, "Burrow scheduler: monitoring %d routine(s) with schedules\n", scheduled)
		}
		for _, r := range routines {
			if r.Schedule != "" {
				tz := r.Timezone
				if tz == "" {
					tz = "local"
				}
				fmt.Fprintf(os.Stderr, "  %s — %s (%s)\n", r.Name, r.Schedule, tz)
			}
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		err = sched.Run(ctx)
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "\nScheduler stopped.")
			return nil
		}
		return err
	},
}

// runRoutine executes a single routine with a fresh config load.
// This replicates the gd routines run execution sequence, ensuring
// credentials are not cached across routine boundaries.
func runRoutine(ctx context.Context, burrowDir string, routine *pipeline.Routine) error {
	cfg, err := config.Load(burrowDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.ResolveEnvVars(cfg)
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	registry, err := buildRegistry(cfg, burrowDir)
	if err != nil {
		return err
	}

	synth, err := buildSynthesizer(routine, cfg)
	if err != nil {
		return fmt.Errorf("configuring synthesizer: %w", err)
	}

	contextDir := filepath.Join(burrowDir, "context")
	ledger, err := bcontext.NewLedger(contextDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not initialize context ledger: %v\n", err)
	}

	reportsDir := filepath.Join(burrowDir, "reports")
	executor := pipeline.NewExecutor(registry, synth, reportsDir)
	if ledger != nil {
		executor.SetLedger(ledger)
	}

	report, err := executor.Run(ctx, routine)
	if err != nil {
		return fmt.Errorf("running routine: %w", err)
	}

	fmt.Fprintf(os.Stderr, "report generated: %s\n", report.Dir)
	return nil
}

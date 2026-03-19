package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/umputun/ralphex/pkg/orchestrator"
)

// orchestrateCmd holds options for the orchestrate subcommand.
type orchestrateCmd struct {
	PlansDir    string        `long:"plans-dir" description:"directory containing plan files (overrides config)"`
	MaxParallel int           `long:"max-parallel" description:"maximum plans in parallel (overrides config)"`
	MaxRetries  int           `long:"max-retries" description:"retry failed plans N times (overrides config)"`
	RetryDelay  time.Duration `long:"retry-delay" description:"delay between retries (overrides config)"`
	FailFast    bool          `long:"fail-fast" description:"stop on first plan failure"`
	DryRun      bool          `long:"dry-run" description:"show execution order without running"`
}

// Execute runs the orchestrate subcommand.
func (cmd *orchestrateCmd) Execute(args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// load config file if exists, CLI flags override
	cfgPath := filepath.Join(".ralphex", "orchestrate.yml")
	cfg, err := orchestrator.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v, using defaults\n", err)
	}

	// CLI flags override config values
	plansDir := cfg.PlansDir
	if cmd.PlansDir != "" {
		plansDir = cmd.PlansDir
	}
	maxParallel := cfg.MaxParallel
	if cmd.MaxParallel > 0 {
		maxParallel = cmd.MaxParallel
	}
	maxRetries := cfg.MaxRetries
	if cmd.MaxRetries > 0 {
		maxRetries = cmd.MaxRetries
	}
	retryDelay := cfg.RetryDelay.Duration
	if cmd.RetryDelay > 0 {
		retryDelay = cmd.RetryDelay
	}
	failFast := cfg.FailFast || cmd.FailFast

	o := &orchestrator.Orchestrator{
		PlansDir:    plansDir,
		MaxParallel: maxParallel,
		MaxRetries:  maxRetries,
		RetryDelay:  retryDelay,
		FailFast:    failFast,
		DryRun:      cmd.DryRun,
	}

	if !cmd.DryRun {
		o.Runner = func(ctx context.Context, planFile string) error {
			return runPlanWithWorktree(ctx, planFile)
		}
	}

	if err := o.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "orchestrate error: %v\n", err)
		return err
	}
	return nil
}

// runPlanWithWorktree executes a single plan using ralphex itself with --worktree.
func runPlanWithWorktree(ctx context.Context, planFile string) error {
	// re-use the existing run() function with worktree mode
	return run(ctx, opts{
		PlanFile: planFile,
		Worktree: true,
	})
}

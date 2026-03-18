package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/umputun/ralphex/pkg/orchestrator"
)

// orchestrateCmd holds options for the orchestrate subcommand.
type orchestrateCmd struct {
	PlansDir    string `long:"plans-dir" default:"docs/plans" description:"directory containing plan files"`
	MaxParallel int    `long:"max-parallel" default:"4" description:"maximum number of plans to run in parallel"`
	FailFast    bool   `long:"fail-fast" description:"stop on first plan failure"`
	DryRun      bool   `long:"dry-run" description:"show execution order without running"`
}

// Execute runs the orchestrate subcommand.
func (cmd *orchestrateCmd) Execute(args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	o := &orchestrator.Orchestrator{
		PlansDir:    cmd.PlansDir,
		MaxParallel: cmd.MaxParallel,
		FailFast:    cmd.FailFast,
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

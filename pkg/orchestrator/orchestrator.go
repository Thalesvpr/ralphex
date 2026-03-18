package orchestrator

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
)

// GraphNode represents a plan in the dependency graph.
type GraphNode struct {
	Entry     PlanEntry
	DependsOn []string // plan names this node depends on
}

// BuildGraph creates a dependency graph from plan entries.
func BuildGraph(entries []PlanEntry) map[string]*GraphNode {
	graph := make(map[string]*GraphNode, len(entries))
	for _, e := range entries {
		node := &GraphNode{
			Entry:     e,
			DependsOn: e.Plan.DependsOn,
		}
		graph[e.Name] = node
	}
	return graph
}

// FindReady returns plan entries that have all dependencies satisfied, are not running, and not completed.
func FindReady(graph map[string]*GraphNode, completed, running map[string]bool) []PlanEntry {
	var ready []PlanEntry
	for name, node := range graph {
		if completed[name] || running[name] {
			continue
		}
		allSatisfied := true
		for _, dep := range node.DependsOn {
			if !completed[dep] {
				allSatisfied = false
				break
			}
		}
		if allSatisfied {
			ready = append(ready, node.Entry)
		}
	}
	// sort for deterministic order
	sort.Slice(ready, func(i, j int) bool { return ready[i].Name < ready[j].Name })
	return ready
}

// Logger is used for orchestrator output.
type Logger interface {
	Printf(format string, args ...any)
}

// stdLogger is a simple stderr logger.
type stdLogger struct{}

func (stdLogger) Printf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}

// PlanRunner is the function signature for running a single plan.
// it receives the plan file path and should block until completion.
type PlanRunner func(ctx context.Context, planFile string) error

// Orchestrator coordinates parallel plan execution respecting dependencies.
type Orchestrator struct {
	PlansDir    string
	MaxParallel int
	FailFast    bool
	DryRun      bool
	RepoDir     string // repo directory for dependency checks; defaults to "."
	Runner      PlanRunner
	Log         Logger
}

// Run executes the orchestration loop.
func (o *Orchestrator) Run(ctx context.Context) error {
	if o.Log == nil {
		o.Log = stdLogger{}
	}
	if o.RepoDir == "" {
		o.RepoDir = "."
	}

	entries, err := LoadPlanFiles(o.PlansDir)
	if err != nil {
		return fmt.Errorf("load plan files: %w", err)
	}
	if len(entries) == 0 {
		o.Log.Printf("no plan files found in %s\n", o.PlansDir)
		return nil
	}

	graph := BuildGraph(entries)

	o.Log.Printf("orchestrator: %d plans loaded\n", len(graph))
	for name, node := range graph {
		if len(node.DependsOn) > 0 {
			o.Log.Printf("  %s depends on: %v\n", name, node.DependsOn)
		} else {
			o.Log.Printf("  %s (no dependencies)\n", name)
		}
	}

	if o.DryRun {
		o.Log.Printf("\ndry run — showing execution order:\n")
		o.printExecutionOrder(graph)
		return nil
	}

	return o.execute(ctx, graph)
}

// printExecutionOrder shows what would run in what order.
func (o *Orchestrator) printExecutionOrder(graph map[string]*GraphNode) {
	completed := make(map[string]bool)
	wave := 1
	for len(completed) < len(graph) {
		ready := FindReady(graph, completed, map[string]bool{})
		if len(ready) == 0 {
			o.Log.Printf("  wave %d: DEADLOCK — circular dependency detected\n", wave)
			break
		}
		names := make([]string, len(ready))
		for i, r := range ready {
			names[i] = r.Name
			completed[r.Name] = true
		}
		o.Log.Printf("  wave %d: %v\n", wave, names)
		wave++
	}
}

// execute runs plans respecting dependencies and parallelism limits.
func (o *Orchestrator) execute(ctx context.Context, graph map[string]*GraphNode) error {
	if o.Runner == nil {
		return fmt.Errorf("no plan runner configured")
	}

	completed := make(map[string]bool)
	running := make(map[string]bool)
	failed := make(map[string]error)
	var mu sync.Mutex

	sem := make(chan struct{}, o.MaxParallel)
	doneCh := make(chan string, len(graph))

	for {
		mu.Lock()
		if len(completed)+len(failed) >= len(graph) {
			mu.Unlock()
			break
		}

		ready := FindReady(graph, completed, running)
		mu.Unlock()

		if len(ready) == 0 {
			// nothing ready — either everything is running or there's a deadlock
			mu.Lock()
			runningCount := len(running)
			mu.Unlock()

			if runningCount == 0 {
				return fmt.Errorf("deadlock: circular dependency detected")
			}

			// wait for a running plan to finish
			select {
			case <-ctx.Done():
				return ctx.Err()
			case name := <-doneCh:
				mu.Lock()
				delete(running, name)
				if o.FailFast && len(failed) > 0 {
					mu.Unlock()
					return fmt.Errorf("plan %s failed (fail-fast enabled)", name)
				}
				mu.Unlock()
				continue
			}
		}

		for _, entry := range ready {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}

			mu.Lock()
			if o.FailFast && len(failed) > 0 {
				mu.Unlock()
				<-sem
				return fmt.Errorf("fail-fast: stopping due to previous failure")
			}
			running[entry.Name] = true
			mu.Unlock()

			go func(e PlanEntry) {
				defer func() {
					<-sem
					doneCh <- e.Name
				}()

				o.Log.Printf("orchestrator: starting %s\n", e.Name)
				if runErr := o.Runner(ctx, e.Path); runErr != nil {
					mu.Lock()
					failed[e.Name] = runErr
					mu.Unlock()
					o.Log.Printf("orchestrator: %s FAILED: %v\n", e.Name, runErr)
					return
				}

				mu.Lock()
				completed[e.Name] = true
				mu.Unlock()
				o.Log.Printf("orchestrator: %s completed\n", e.Name)
			}(entry)
		}

		// wait for at least one to finish before re-checking
		select {
		case <-ctx.Done():
			return ctx.Err()
		case name := <-doneCh:
			mu.Lock()
			delete(running, name)
			if o.FailFast && len(failed) > 0 {
				mu.Unlock()
				return fmt.Errorf("plan %s failed (fail-fast enabled)", name)
			}
			mu.Unlock()
		}
	}

	if len(failed) > 0 {
		var msg string
		for name, err := range failed {
			msg += fmt.Sprintf("\n  %s: %v", name, err)
		}
		return fmt.Errorf("%d plan(s) failed:%s", len(failed), msg)
	}

	o.Log.Printf("orchestrator: all %d plans completed successfully\n", len(completed))
	return nil
}

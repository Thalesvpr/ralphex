package orchestrator

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
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

// planResult is sent over the done channel when a plan finishes.
type planResult struct {
	Name string
	Err  error
}

// Orchestrator coordinates parallel plan execution respecting dependencies.
type Orchestrator struct {
	PlansDir    string
	MaxParallel int
	MaxRetries  int  // retry failed plans up to this many times (0 = no retry)
	RetryDelay  time.Duration
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
	if o.MaxRetries < 0 {
		o.MaxRetries = 0
	}
	if o.RetryDelay == 0 {
		o.RetryDelay = 30 * time.Second
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
	for _, name := range sortedKeys(graph) {
		node := graph[name]
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
	retries := make(map[string]int)
	var mu sync.Mutex

	sem := make(chan struct{}, o.MaxParallel)
	doneCh := make(chan planResult, len(graph))

	totalPlans := len(graph)

	for {
		mu.Lock()
		doneCount := len(completed) + len(failed)
		mu.Unlock()

		if doneCount >= totalPlans {
			break
		}

		mu.Lock()
		ready := FindReady(graph, completed, running)
		mu.Unlock()

		if len(ready) == 0 {
			mu.Lock()
			runningCount := len(running)
			mu.Unlock()

			if runningCount == 0 {
				// check if all remaining are failed (not deadlock)
				mu.Lock()
				remainingFailed := len(failed)
				mu.Unlock()
				if remainingFailed > 0 {
					break // all remaining plans depend on failed ones
				}
				return fmt.Errorf("deadlock: circular dependency detected")
			}

			// wait for a running plan to finish
			if err := o.handleDone(ctx, doneCh, &mu, completed, running, failed, retries, graph, sem); err != nil {
				return err
			}
			continue
		}

		// launch ready plans
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
				break
			}
			running[entry.Name] = true
			mu.Unlock()

			go func(e PlanEntry) {
				defer func() { <-sem }()

				o.Log.Printf("orchestrator: starting %s\n", e.Name)
				runErr := o.Runner(ctx, e.Path)
				doneCh <- planResult{Name: e.Name, Err: runErr}
			}(entry)
		}

		// wait for at least one to finish before re-checking
		if err := o.handleDone(ctx, doneCh, &mu, completed, running, failed, retries, graph, sem); err != nil {
			return err
		}
	}

	if len(failed) > 0 {
		var msg string
		for name, ferr := range failed {
			msg += fmt.Sprintf("\n  %s: %v", name, ferr)
		}
		return fmt.Errorf("%d plan(s) failed:%s", len(failed), msg)
	}

	o.Log.Printf("orchestrator: all %d plans completed successfully\n", len(completed))
	return nil
}

// handleDone processes one completion from the done channel.
func (o *Orchestrator) handleDone(ctx context.Context, doneCh chan planResult,
	mu *sync.Mutex, completed, running map[string]bool, failed map[string]error,
	retries map[string]int, graph map[string]*GraphNode, sem chan struct{}) error {

	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-doneCh:
		mu.Lock()
		delete(running, result.Name)

		if result.Err != nil {
			attempt := retries[result.Name]
			if attempt < o.MaxRetries {
				retries[result.Name] = attempt + 1
				mu.Unlock()

				o.Log.Printf("orchestrator: %s FAILED (attempt %d/%d): %v — retrying in %s\n",
					result.Name, attempt+1, o.MaxRetries+1, result.Err, o.RetryDelay)

				// retry after delay
				go func(name string) {
					select {
					case <-ctx.Done():
						return
					case <-time.After(o.RetryDelay):
					}

					sem <- struct{}{}

					mu.Lock()
					entry := graph[name].Entry
					running[name] = true
					mu.Unlock()

					defer func() { <-sem }()

					o.Log.Printf("orchestrator: retrying %s\n", name)
					runErr := o.Runner(ctx, entry.Path)
					doneCh <- planResult{Name: name, Err: runErr}
				}(result.Name)

				return nil
			}

			failed[result.Name] = result.Err
			mu.Unlock()
			o.Log.Printf("orchestrator: %s FAILED (all %d attempts exhausted): %v\n",
				result.Name, o.MaxRetries+1, result.Err)

			if o.FailFast {
				return fmt.Errorf("plan %s failed (fail-fast enabled): %w", result.Name, result.Err)
			}
			return nil
		}

		completed[result.Name] = true
		mu.Unlock()
		o.Log.Printf("orchestrator: %s completed\n", result.Name)
		return nil
	}
}

// sortedKeys returns map keys in sorted order.
func sortedKeys(m map[string]*GraphNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

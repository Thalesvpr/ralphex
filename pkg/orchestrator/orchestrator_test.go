package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGraph(t *testing.T) {
	t.Run("no dependencies", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "01-first.md", "# First\n\n### Task 1: T\n- [ ] x")
		writeFile(t, dir, "02-second.md", "# Second\n\n### Task 1: T\n- [ ] x")

		entries, err := LoadPlanFiles(dir)
		require.NoError(t, err)
		graph := BuildGraph(entries)
		assert.Len(t, graph, 2)
		for _, node := range graph {
			assert.Empty(t, node.DependsOn)
		}
	})

	t.Run("linear dependency chain", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "01-first.md", "# First\n\n### Task 1: T\n- [ ] x")
		writeFile(t, dir, "02-second.md", "---\ndepends_on:\n  - 01-first\n---\n# Second\n\n### Task 1: T\n- [ ] x")
		writeFile(t, dir, "03-third.md", "---\ndepends_on:\n  - 02-second\n---\n# Third\n\n### Task 1: T\n- [ ] x")

		entries, err := LoadPlanFiles(dir)
		require.NoError(t, err)
		graph := BuildGraph(entries)

		assert.Empty(t, graph["01-first"].DependsOn)
		assert.Equal(t, []string{"01-first"}, graph["02-second"].DependsOn)
		assert.Equal(t, []string{"02-second"}, graph["03-third"].DependsOn)
	})
}

func TestFindReady(t *testing.T) {
	t.Run("all independent plans are ready", func(t *testing.T) {
		graph := map[string]*GraphNode{
			"a": {Entry: PlanEntry{Name: "a"}, DependsOn: nil},
			"b": {Entry: PlanEntry{Name: "b"}, DependsOn: nil},
		}
		completed := map[string]bool{}
		running := map[string]bool{}
		ready := FindReady(graph, completed, running)
		assert.Len(t, ready, 2)
	})

	t.Run("dependent plan not ready until dep completed", func(t *testing.T) {
		graph := map[string]*GraphNode{
			"a": {Entry: PlanEntry{Name: "a"}, DependsOn: nil},
			"b": {Entry: PlanEntry{Name: "b"}, DependsOn: []string{"a"}},
		}

		// a not completed
		ready := FindReady(graph, map[string]bool{}, map[string]bool{})
		assert.Len(t, ready, 1)
		assert.Equal(t, "a", ready[0].Name)

		// a completed
		ready = FindReady(graph, map[string]bool{"a": true}, map[string]bool{})
		assert.Len(t, ready, 1)
		assert.Equal(t, "b", ready[0].Name)
	})

	t.Run("excludes running and completed", func(t *testing.T) {
		graph := map[string]*GraphNode{
			"a": {Entry: PlanEntry{Name: "a"}, DependsOn: nil},
			"b": {Entry: PlanEntry{Name: "b"}, DependsOn: nil},
			"c": {Entry: PlanEntry{Name: "c"}, DependsOn: nil},
		}
		completed := map[string]bool{"a": true}
		running := map[string]bool{"b": true}
		ready := FindReady(graph, completed, running)
		assert.Len(t, ready, 1)
		assert.Equal(t, "c", ready[0].Name)
	})

	t.Run("diamond dependency", func(t *testing.T) {
		// a -> b, a -> c, b+c -> d
		graph := map[string]*GraphNode{
			"a": {Entry: PlanEntry{Name: "a"}, DependsOn: nil},
			"b": {Entry: PlanEntry{Name: "b"}, DependsOn: []string{"a"}},
			"c": {Entry: PlanEntry{Name: "c"}, DependsOn: []string{"a"}},
			"d": {Entry: PlanEntry{Name: "d"}, DependsOn: []string{"b", "c"}},
		}

		// only a ready initially
		ready := FindReady(graph, map[string]bool{}, map[string]bool{})
		assert.Len(t, ready, 1)
		assert.Equal(t, "a", ready[0].Name)

		// a done -> b and c ready
		ready = FindReady(graph, map[string]bool{"a": true}, map[string]bool{})
		assert.Len(t, ready, 2)

		// a, b, c done -> d ready
		ready = FindReady(graph, map[string]bool{"a": true, "b": true, "c": true}, map[string]bool{})
		assert.Len(t, ready, 1)
		assert.Equal(t, "d", ready[0].Name)
	})
}

func TestOrchestratorDryRun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "01-first.md", "# First\n\n### Task 1: T\n- [ ] x")
	writeFile(t, dir, "02-second.md", "---\ndepends_on:\n  - 01-first\n---\n# Second\n\n### Task 1: T\n- [ ] x")
	writeFile(t, dir, "03-parallel.md", "# Parallel\n\n### Task 1: T\n- [ ] x")

	o := &Orchestrator{
		PlansDir:    dir,
		MaxParallel: 4,
		DryRun:      true,
	}

	err := o.Run(context.Background())
	require.NoError(t, err)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

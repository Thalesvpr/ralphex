package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupGitRepo creates a temporary git repo with an initial commit and returns its path.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}
	return dir
}

// createMergedBranch creates a branch, adds a commit, and merges it into master/main.
func createMergedBranch(t *testing.T, repoDir, branchName string) {
	t.Helper()
	cmds := [][]string{
		{"git", "checkout", "-b", branchName},
		{"git", "commit", "--allow-empty", "-m", "work on " + branchName},
		{"git", "checkout", "-"},
		{"git", "merge", branchName, "--no-ff", "-m", "merge " + branchName},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}
}

func TestDepsAreSatisfied(t *testing.T) {
	t.Run("empty deps always satisfied", func(t *testing.T) {
		dir := setupGitRepo(t)
		satisfied, err := DepsAreSatisfied(nil, dir)
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("satisfied when branch is merged", func(t *testing.T) {
		dir := setupGitRepo(t)
		createMergedBranch(t, dir, "professional-scheduling")

		satisfied, err := DepsAreSatisfied([]string{"07-professional-scheduling"}, dir)
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("not satisfied when branch is not merged", func(t *testing.T) {
		dir := setupGitRepo(t)

		// create unmerged branch
		cmds := [][]string{
			{"git", "checkout", "-b", "professional-scheduling"},
			{"git", "commit", "--allow-empty", "-m", "work"},
			{"git", "checkout", "-"},
		}
		for _, args := range cmds {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "cmd %v failed: %s", args, out)
		}

		satisfied, err := DepsAreSatisfied([]string{"07-professional-scheduling"}, dir)
		require.NoError(t, err)
		assert.False(t, satisfied)
	})

	t.Run("not satisfied when branch does not exist", func(t *testing.T) {
		dir := setupGitRepo(t)
		satisfied, err := DepsAreSatisfied([]string{"07-nonexistent"}, dir)
		require.NoError(t, err)
		assert.False(t, satisfied)
	})

	t.Run("multiple deps all satisfied", func(t *testing.T) {
		dir := setupGitRepo(t)
		createMergedBranch(t, dir, "professional-scheduling")
		createMergedBranch(t, dir, "refactor-permissions-context")

		deps := []string{"07-professional-scheduling", "08-refactor-permissions-context"}
		satisfied, err := DepsAreSatisfied(deps, dir)
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("multiple deps one not satisfied", func(t *testing.T) {
		dir := setupGitRepo(t)
		createMergedBranch(t, dir, "professional-scheduling")

		deps := []string{"07-professional-scheduling", "08-refactor-permissions-context"}
		satisfied, err := DepsAreSatisfied(deps, dir)
		require.NoError(t, err)
		assert.False(t, satisfied)
	})
}

func TestNormalizeDep(t *testing.T) {
	tests := []struct {
		name string
		dep  string
		want string
	}{
		{"strips numeric prefix", "07-professional-scheduling", "professional-scheduling"},
		{"strips multi-digit prefix", "123-some-feature", "some-feature"},
		{"no prefix", "some-feature", "some-feature"},
		{"only number", "07", "07"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeDep(tt.dep))
		})
	}
}

func TestLoadPlanFiles(t *testing.T) {
	t.Run("loads plan files from directory", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "01-first.md"), []byte("# First\n\n### Task 1: T\n- [ ] x"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "02-second.md"), []byte("---\ndepends_on:\n  - 01-first\n---\n# Second\n\n### Task 1: T\n- [ ] x"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "not-a-plan.txt"), []byte("ignore"), 0o600))

		plans, err := LoadPlanFiles(dir)
		require.NoError(t, err)
		require.Len(t, plans, 2)

		// find plan with dependencies
		var withDeps *PlanEntry
		for i := range plans {
			if plans[i].Name == "02-second" {
				withDeps = &plans[i]
			}
		}
		require.NotNil(t, withDeps)
		assert.Equal(t, []string{"01-first"}, withDeps.Plan.DependsOn)
	})
}

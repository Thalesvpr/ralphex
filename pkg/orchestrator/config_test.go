package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("defaults when file missing", func(t *testing.T) {
		cfg, err := LoadConfig("/nonexistent/path.yml")
		require.NoError(t, err)
		assert.Equal(t, "plans", cfg.Source)
		assert.Equal(t, "docs/plans", cfg.PlansDir)
		assert.Equal(t, 4, cfg.MaxParallel)
		assert.Equal(t, 2, cfg.MaxRetries)
		assert.Equal(t, 30*time.Second, cfg.RetryDelay.Duration)
		assert.False(t, cfg.FailFast)
	})

	t.Run("reads plans config", func(t *testing.T) {
		dir := t.TempDir()
		content := `
source: plans
plans_dir: .ralph/plans
max_parallel: 2
max_retries: 5
retry_delay: 1m
fail_fast: true
`
		path := filepath.Join(dir, "orchestrate.yml")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		cfg, err := LoadConfig(path)
		require.NoError(t, err)
		assert.Equal(t, "plans", cfg.Source)
		assert.Equal(t, ".ralph/plans", cfg.PlansDir)
		assert.Equal(t, 2, cfg.MaxParallel)
		assert.Equal(t, 5, cfg.MaxRetries)
		assert.Equal(t, time.Minute, cfg.RetryDelay.Duration)
		assert.True(t, cfg.FailFast)
	})

	t.Run("reads issues config", func(t *testing.T) {
		dir := t.TempDir()
		content := `
source: issues
issues:
  label: ralphex
  auto_plan: true
  repo: Thalesvpr/trinteum-backend
`
		path := filepath.Join(dir, "orchestrate.yml")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		cfg, err := LoadConfig(path)
		require.NoError(t, err)
		assert.Equal(t, "issues", cfg.Source)
		assert.Equal(t, "ralphex", cfg.Issues.Label)
		assert.True(t, cfg.Issues.AutoPlan)
		assert.Equal(t, "Thalesvpr/trinteum-backend", cfg.Issues.Repo)
	})

	t.Run("invalid yaml returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yml")
		require.NoError(t, os.WriteFile(path, []byte(":::invalid"), 0o644))

		_, err := LoadConfig(path)
		assert.Error(t, err)
	})
}

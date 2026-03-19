package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCmd_Execute(t *testing.T) {
	t.Run("creates basic structure", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(dir))
		defer func() { _ = os.Chdir(origDir) }()

		// init git repo so gitignore works
		cmd := &initCmd{}
		err := cmd.Execute(nil)
		require.NoError(t, err)

		assert.DirExists(t, filepath.Join(dir, ".ralphex"))
		assert.DirExists(t, filepath.Join(dir, ".ralphex", "progress"))
		assert.DirExists(t, filepath.Join(dir, ".ralphex", "logs"))
		assert.FileExists(t, filepath.Join(dir, "RALPH.md"))
		assert.FileExists(t, filepath.Join(dir, ".gitignore"))
	})

	t.Run("creates orchestration structure", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(dir))
		defer func() { _ = os.Chdir(origDir) }()

		cmd := &initCmd{Orchestrate: true, PlansDir: ".ralph/plans"}
		err := cmd.Execute(nil)
		require.NoError(t, err)

		assert.DirExists(t, filepath.Join(dir, ".ralph", "plans"))

		content, err := os.ReadFile(filepath.Join(dir, "RALPH.md"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "depends_on")
	})

	t.Run("does not overwrite existing RALPH.md", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(dir))
		defer func() { _ = os.Chdir(origDir) }()

		existing := "my existing content"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "RALPH.md"), []byte(existing), 0o644))

		cmd := &initCmd{}
		err := cmd.Execute(nil)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, "RALPH.md"))
		require.NoError(t, err)
		assert.Equal(t, existing, string(content))
	})

	t.Run("gitignore entries are idempotent", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		require.NoError(t, os.Chdir(dir))
		defer func() { _ = os.Chdir(origDir) }()

		cmd := &initCmd{}
		require.NoError(t, cmd.Execute(nil))
		require.NoError(t, cmd.Execute(nil)) // second run

		content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		require.NoError(t, err)

		// each entry should appear only once
		count := 0
		for _, line := range splitLines(string(content)) {
			if line == ".ralphex/worktrees/" {
				count++
			}
		}
		assert.Equal(t, 1, count)
	})
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range []byte(s) {
		_ = line
	}
	// simple split
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

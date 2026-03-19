package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CleanupConfig holds settings for auto-cleanup.
type CleanupConfig struct {
	LogRetentionDays int // delete logs older than this (0 = disabled)
	PruneWorktrees   bool
	RepoDir          string
}

// Cleanup runs pre-execution maintenance: prune stale worktrees, rotate old logs.
func Cleanup(cfg CleanupConfig, log Logger) {
	if log == nil {
		log = stdLogger{}
	}

	if cfg.PruneWorktrees {
		pruneWorktrees(cfg.RepoDir, log)
	}

	if cfg.LogRetentionDays > 0 {
		ralphexDir := filepath.Join(cfg.RepoDir, ".ralphex")
		cutoff := time.Now().AddDate(0, 0, -cfg.LogRetentionDays)
		cleanOldFiles(filepath.Join(ralphexDir, "logs"), cutoff, log)
		cleanOldFiles(filepath.Join(ralphexDir, "progress"), cutoff, log)
	}
}

// pruneWorktrees removes stale git worktrees.
func pruneWorktrees(repoDir string, log Logger) {
	// remove orphaned worktree dirs
	wtDir := filepath.Join(repoDir, ".ralphex", "worktrees")
	entries, err := os.ReadDir(wtDir)
	if err != nil {
		return // no worktrees dir, nothing to do
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p := filepath.Join(wtDir, entry.Name())
		// check if it's a valid worktree by looking for .git file
		if _, err := os.Stat(filepath.Join(p, ".git")); os.IsNotExist(err) {
			log.Printf("cleanup: removing orphaned worktree dir %s\n", entry.Name())
			_ = os.RemoveAll(p)
		}
	}

	// git worktree prune
	cmd := exec.Command("git", "worktree", "prune")
	if repoDir != "" && repoDir != "." {
		cmd.Dir = repoDir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("cleanup: git worktree prune: %v (%s)\n", err, strings.TrimSpace(string(out)))
	}
}

// cleanOldFiles removes files older than cutoff in a directory.
func cleanOldFiles(dir string, cutoff time.Time, log Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			p := filepath.Join(dir, entry.Name())
			if err := os.Remove(p); err == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		log.Printf("cleanup: removed %d old files from %s\n", removed, dir)
	}
}

// RecordProblem appends a problem entry to .ralphex/problems.md.
func RecordProblem(repoDir, planName string, err error, context string) {
	problemsPath := filepath.Join(repoDir, ".ralphex", "problems.md")

	// ensure directory exists
	_ = os.MkdirAll(filepath.Dir(problemsPath), 0o755)

	f, ferr := os.OpenFile(problemsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if ferr != nil {
		return
	}
	defer f.Close()

	// write header if file is new
	info, _ := f.Stat()
	if info != nil && info.Size() == 0 {
		fmt.Fprintf(f, "# Problems encountered during ralphex execution\n\n")
		fmt.Fprintf(f, "Auto-generated. Each entry is a blocker that required intervention or caused a plan to fail.\n\n---\n\n")
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "### %s — %s\n\n", timestamp, planName)
	fmt.Fprintf(f, "**Error:** %v\n\n", err)
	if context != "" {
		fmt.Fprintf(f, "**Context:** %s\n\n", context)
	}
	fmt.Fprintf(f, "---\n\n")
}

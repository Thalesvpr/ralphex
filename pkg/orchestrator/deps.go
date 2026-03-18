// Package orchestrator provides dependency-aware plan execution.
package orchestrator

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/umputun/ralphex/pkg/plan"
)

// numPrefixRe matches a leading numeric prefix followed by a dash (e.g., "07-").
var numPrefixRe = regexp.MustCompile(`^\d+-`)

// PlanEntry represents a plan file with its parsed metadata.
type PlanEntry struct {
	Name string     // filename without extension (e.g., "07-professional-scheduling")
	Path string     // absolute path to the plan file
	Plan *plan.Plan // parsed plan
}

// normalizeDep strips a leading numeric prefix from a dependency name.
// e.g., "07-professional-scheduling" -> "professional-scheduling".
func normalizeDep(dep string) string {
	result := numPrefixRe.ReplaceAllString(dep, "")
	if result == "" {
		return dep
	}
	return result
}

// DepsAreSatisfied checks whether all dependencies are satisfied.
// a dependency is satisfied when a branch matching the normalized name exists and is merged into the default branch.
func DepsAreSatisfied(deps []string, repoDir string) (bool, error) {
	if len(deps) == 0 {
		return true, nil
	}

	// get default branch name
	defaultBranch, err := getDefaultBranch(repoDir)
	if err != nil {
		return false, fmt.Errorf("get default branch: %w", err)
	}

	// get merged branches
	cmd := exec.Command("git", "branch", "--merged", defaultBranch)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git branch --merged: %w", err)
	}

	merged := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		branch := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "* "))
		if branch != "" {
			merged[branch] = true
		}
	}

	for _, dep := range deps {
		branchName := normalizeDep(dep)
		if !merged[branchName] {
			return false, nil
		}
	}
	return true, nil
}

// getDefaultBranch detects the default branch (main or master).
func getDefaultBranch(repoDir string) (string, error) {
	// try common default branch names
	for _, name := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", name)
		cmd.Dir = repoDir
		if err := cmd.Run(); err == nil {
			return name, nil
		}
	}

	// fallback: use HEAD
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("detect default branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// LoadPlanFiles reads all .md files from a directory and parses them.
func LoadPlanFiles(dir string) ([]PlanEntry, error) {
	pattern := filepath.Join(dir, "*.md")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob plan files: %w", err)
	}

	var entries []PlanEntry
	for _, f := range files {
		p, parseErr := plan.ParsePlanFile(f)
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", f, parseErr)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		entries = append(entries, PlanEntry{
			Name: name,
			Path: f,
			Plan: p,
		})
	}
	return entries, nil
}

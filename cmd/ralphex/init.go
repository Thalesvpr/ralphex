package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

)

// initCmd holds options for the init subcommand.
type initCmd struct {
	Orchestrate bool   `long:"orchestrate" description:"also create orchestration config and plans directory"`
	PlansDir    string `long:"plans-dir" default:".ralph/plans" description:"plans directory for orchestration"`
}

// Execute runs the init subcommand.
func (cmd *initCmd) Execute(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	fmt.Println("initializing ralphex...")

	// create .ralphex/ structure
	dirs := []string{
		".ralphex",
		".ralphex/progress",
		".ralphex/logs",
	}
	if cmd.Orchestrate {
		dirs = append(dirs, cmd.PlansDir)
	}

	for _, d := range dirs {
		p := filepath.Join(cwd, d)
		if err := os.MkdirAll(p, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
		fmt.Printf("  created %s/\n", d)
	}

	// create RALPH.md template if not exists
	ralphPath := filepath.Join(cwd, "RALPH.md")
	if _, err := os.Stat(ralphPath); os.IsNotExist(err) {
		if writeErr := os.WriteFile(ralphPath, []byte(ralphTemplate(cmd.Orchestrate, cmd.PlansDir)), 0o644); writeErr != nil {
			return fmt.Errorf("create RALPH.md: %w", writeErr)
		}
		fmt.Println("  created RALPH.md")
	} else {
		fmt.Println("  RALPH.md already exists, skipping")
	}

	// update .gitignore
	if err := ensureGitignoreEntries(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not update .gitignore: %v\n", err)
	}

	if cmd.Orchestrate {
		// create orchestration config if not exists
		configPath := filepath.Join(cwd, ".ralphex", "orchestrate.yml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			if writeErr := os.WriteFile(configPath, []byte(orchestrateConfigTemplate(cmd.PlansDir)), 0o644); writeErr != nil {
				return fmt.Errorf("create orchestrate.yml: %w", writeErr)
			}
			fmt.Println("  created .ralphex/orchestrate.yml")
		}

		fmt.Printf("\norchestration enabled. add plans to %s/\n", cmd.PlansDir)
		fmt.Println("run: ralphex orchestrate --plans-dir", cmd.PlansDir)
	}

	fmt.Println("\ndone. edit RALPH.md with your project context.")
	return nil
}

// ralphTemplate generates the RALPH.md template content.
func ralphTemplate(orchestrate bool, plansDir string) string {
	var sb strings.Builder
	sb.WriteString(`# RALPH.md — Project Context for Ralphex

<!-- Edit this file with your project's context, rules, and conventions. -->
<!-- Ralphex reads this file to understand your project before executing plans. -->

## Project

<!-- Describe your project: what it does, what stack it uses, how it's structured. -->

## Rules

<!-- Critical rules that must never be violated. Examples: -->
<!-- - Never use DateTime.UtcNow directly (use injected clock) -->
<!-- - Always write tests before implementation (TDD) -->
<!-- - Never add Co-Authored-By in commits -->

## Commands

<!-- How to build, test, and validate changes. -->
<!-- - Build: ... -->
<!-- - Test: ... -->
<!-- - Lint: ... -->
`)

	if orchestrate {
		sb.WriteString(fmt.Sprintf(`
## Plans

Plans are in %s/. Each plan is a markdown file with tasks in checkboxes.
Plans can declare dependencies via YAML frontmatter:

`+"```yaml"+`
---
depends_on:
  - 01-setup
  - 02-backend
---
`+"```"+`

Run all plans: ralphex orchestrate --plans-dir %s
`, plansDir, plansDir))
	}

	return sb.String()
}

// orchestrateConfigTemplate generates the orchestration config YAML.
func orchestrateConfigTemplate(plansDir string) string {
	return fmt.Sprintf(`# Ralphex orchestration config
# This file controls how ralphex orchestrate behaves in this project.

# Where plans come from
source: plans  # "plans" = local markdown files, "issues" = GitHub issues

# Local plans config (when source: plans)
plans_dir: %s

# GitHub issues config (when source: issues)
# issues:
#   label: ralphex        # only issues with this label
#   auto_plan: true       # generate plan .md from issue before executing
#   repo: owner/repo      # defaults to current git remote

# Execution settings
max_parallel: 4           # max plans running simultaneously
max_retries: 2            # retry failed plans up to N times
retry_delay: 30s          # delay between retries
fail_fast: false          # stop everything on first failure
`, plansDir)
}

// ensureGitignoreEntries adds ralphex entries to .gitignore if missing.
func ensureGitignoreEntries(cwd string) error {
	gitignorePath := filepath.Join(cwd, ".gitignore")

	var existing string
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	entries := []string{
		".ralphex/worktrees/",
		".ralphex/progress/",
		".ralphex/logs/",
	}

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(existing, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	// add newline separator if file doesn't end with one
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	content := "\n# ralphex\n"
	for _, entry := range toAdd {
		content += entry + "\n"
	}
	_, err = f.WriteString(content)
	fmt.Println("  updated .gitignore")
	return err
}

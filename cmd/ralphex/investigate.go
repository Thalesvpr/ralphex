package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// investigateCmd holds options for the investigate subcommand.
type investigateCmd struct {
	Scope     string `long:"scope" default:"all" description:"what to investigate: all, issues, codebase, plans"`
	Label     string `long:"label" default:"ralphex" description:"GitHub issue label to work with"`
	Repo      string `long:"repo" description:"GitHub repo (owner/repo), defaults to current"`
	PlansDir  string `long:"plans-dir" default:"docs/plans" description:"directory for generated/refined plans"`
	MaxCycles int    `long:"max-cycles" default:"1" description:"investigation cycles (0 = continuous)"`
}

// Execute runs the investigate subcommand.
func (cmd *investigateCmd) Execute(args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	prompt := cmd.buildPrompt()

	fmt.Fprintf(os.Stderr, "investigate: scope=%s, label=%s, plans=%s\n", cmd.Scope, cmd.Label, cmd.PlansDir)

	cycles := 0
	for {
		if cmd.MaxCycles > 0 && cycles >= cmd.MaxCycles {
			break
		}

		if err := run(ctx, opts{
			PlanFile:        "", // no plan file — investigate mode uses prompt directly
			PlanDescription: prompt,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "investigate cycle %d error: %v\n", cycles+1, err)
			if cmd.MaxCycles == 1 {
				return err
			}
		}

		cycles++
		fmt.Fprintf(os.Stderr, "investigate: cycle %d complete\n", cycles)
	}

	return nil
}

// buildPrompt generates the investigation prompt based on scope.
func (cmd *investigateCmd) buildPrompt() string {
	base := `You are an autonomous tech lead investigating this project. Your job is to analyze the codebase, identify improvements, and create actionable plans.

IMPORTANT:
- Do NOT make code changes. Only create/update plans and issues.
- Write plans in markdown with ### Task N: headers and - [ ] checkboxes.
- Be specific and actionable — each task should be implementable by another agent.
`

	switch cmd.Scope {
	case "issues":
		return base + fmt.Sprintf(`
SCOPE: GitHub Issues

1. Run: gh issue list --label %q --state open --repo %s
2. For each issue without a plan file in %s/:
   - Read the issue body
   - Investigate the codebase to understand what needs to change
   - Create a detailed plan file in %s/ with tasks and checkboxes
   - The plan filename should be: {issue-number}-{slugified-title}.md
3. For each issue that already has a plan:
   - Review the plan for completeness
   - Add missing tasks or refine existing ones
   - Update the plan file if needed
4. Look for dependency relationships between issues
   - Add depends_on frontmatter where appropriate
   - Add "depends:{plan-name}" labels on GitHub issues

After completing investigation, output <<<RALPHEX:COMPLETED>>>
`, cmd.Label, cmd.repoOrCurrent(), cmd.PlansDir, cmd.PlansDir)

	case "codebase":
		return base + fmt.Sprintf(`
SCOPE: Codebase Analysis

1. Analyze the project structure and code quality
2. Identify:
   - Bugs or potential issues
   - Missing test coverage
   - Code duplication
   - Performance concerns
   - Security vulnerabilities
   - Technical debt
3. For each finding, create a GitHub issue:
   - gh issue create --title "..." --body "..." --label %q
   - Include enough context for another agent to implement a fix
4. If significant enough, also create a plan file in %s/

After completing investigation, output <<<RALPHEX:COMPLETED>>>
`, cmd.Label, cmd.PlansDir)

	case "plans":
		return base + fmt.Sprintf(`
SCOPE: Plan Refinement

1. Read all plan files in %s/
2. For each plan:
   - Verify tasks are clear and actionable
   - Check that dependencies are correctly declared
   - Verify the plan follows TDD (tests before implementation)
   - Ensure validation commands are included
   - Check that the plan doesn't break existing functionality
3. Refine any plan that needs improvement
4. Create dependency frontmatter where missing

After completing investigation, output <<<RALPHEX:COMPLETED>>>
`, cmd.PlansDir)

	default: // "all"
		return base + fmt.Sprintf(`
SCOPE: Full Investigation

Phase 1 — Issues:
- List open issues with label %q
- Create/refine plan files for each issue in %s/

Phase 2 — Codebase:
- Analyze code quality, identify tech debt, bugs, missing tests
- Create issues for significant findings: gh issue create --label %q

Phase 3 — Plans:
- Review all plans in %s/ for quality and completeness
- Add dependency frontmatter where needed
- Ensure all plans are actionable

After completing all phases, output <<<RALPHEX:COMPLETED>>>
`, cmd.Label, cmd.PlansDir, cmd.Label, cmd.PlansDir)
	}
}

func (cmd *investigateCmd) repoOrCurrent() string {
	if cmd.Repo != "" {
		return cmd.Repo
	}
	return "(current repo)"
}

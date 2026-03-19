package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitHubIssue represents a GitHub issue with relevant fields.
type GitHubIssue struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
	State  string   `json:"state"`
}

// issueJSON is the raw JSON shape from gh CLI.
type issueJSON struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// FetchIssues retrieves open issues with a specific label from a GitHub repo.
// uses the gh CLI. repo format: "owner/repo" or empty for current repo.
func FetchIssues(repo, label string) ([]GitHubIssue, error) {
	args := []string{"issue", "list", "--state", "open", "--json", "number,title,body,state,labels", "--limit", "100"}
	if label != "" {
		args = append(args, "--label", label)
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}

	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh issue list: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("gh issue list: %w", err)
	}

	var raw []issueJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse issues json: %w", err)
	}

	issues := make([]GitHubIssue, len(raw))
	for i, r := range raw {
		labels := make([]string, len(r.Labels))
		for j, l := range r.Labels {
			labels[j] = l.Name
		}
		issues[i] = GitHubIssue{
			Number: r.Number,
			Title:  r.Title,
			Body:   r.Body,
			Labels: labels,
			State:  r.State,
		}
	}
	return issues, nil
}

// IssueToPlanFile generates a plan .md file from a GitHub issue.
// writes to plansDir and returns the file path.
func IssueToPlanFile(issue GitHubIssue, plansDir string) (string, error) {
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return "", fmt.Errorf("create plans dir: %w", err)
	}

	filename := fmt.Sprintf("%02d-%s.md", issue.Number, slugify(issue.Title))
	path := filepath.Join(plansDir, filename)

	// if plan file already exists, don't overwrite (may have been manually refined)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	content := formatIssuePlan(issue)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write plan file: %w", err)
	}

	return path, nil
}

// CloseIssue closes a GitHub issue after successful plan execution.
func CloseIssue(repo string, issueNumber int) error {
	args := []string{"issue", "close", fmt.Sprintf("%d", issueNumber)}
	if repo != "" {
		args = append(args, "--repo", repo)
	}

	cmd := exec.Command("gh", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh issue close #%d: %s", issueNumber, string(out))
	}
	return nil
}

// formatIssuePlan formats a GitHub issue body as a ralphex plan.
// if the body already has ### Task headers, use it as-is.
// otherwise wrap it in a basic plan structure.
func formatIssuePlan(issue GitHubIssue) string {
	body := strings.TrimSpace(issue.Body)

	// check if body already has plan structure
	if strings.Contains(body, "### Task") || strings.Contains(body, "### Iteration") {
		// extract depends_on from labels like "depends:07-feature"
		deps := extractDepsFromLabels(issue.Labels)
		header := fmt.Sprintf("# %s\n\nIssue: #%d\n\n", issue.Title, issue.Number)
		if len(deps) > 0 {
			header = fmt.Sprintf("---\ndepends_on:\n")
			for _, d := range deps {
				header += fmt.Sprintf("  - %s\n", d)
			}
			header += fmt.Sprintf("---\n\n# %s\n\nIssue: #%d\n\n", issue.Title, issue.Number)
		}
		return header + body + "\n"
	}

	// wrap in basic plan structure
	var sb strings.Builder
	deps := extractDepsFromLabels(issue.Labels)
	if len(deps) > 0 {
		sb.WriteString("---\ndepends_on:\n")
		for _, d := range deps {
			sb.WriteString(fmt.Sprintf("  - %s\n", d))
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString(fmt.Sprintf("# %s\n\nIssue: #%d\n\n", issue.Title, issue.Number))
	sb.WriteString(body)
	sb.WriteString("\n\n### Task 1: Investigate and implement\n")
	sb.WriteString("- [ ] Read the issue description and understand requirements\n")
	sb.WriteString("- [ ] Implement the changes described above\n")
	sb.WriteString("- [ ] Ensure build passes\n")
	sb.WriteString("- [ ] Ensure tests pass\n")

	return sb.String()
}

// extractDepsFromLabels finds labels like "depends:07-feature" and returns dep names.
func extractDepsFromLabels(labels []string) []string {
	var deps []string
	for _, l := range labels {
		if after, found := strings.CutPrefix(l, "depends:"); found {
			deps = append(deps, strings.TrimSpace(after))
		}
	}
	return deps
}

// slugify converts a title to a filename-safe slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, s)

	// collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")

	// truncate
	if len(s) > 60 {
		s = s[:60]
		s = strings.TrimRight(s, "-")
	}
	return s
}

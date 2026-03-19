package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Add user authentication", "add-user-authentication"},
		{"Fix bug #123", "fix-bug-123"},
		{"  spaces  everywhere  ", "spaces-everywhere"},
		{"Special!@#$Characters", "specialcharacters"},
		{"already-slugified", "already-slugified"},
		{"UPPERCASE", "uppercase"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, slugify(tt.input))
		})
	}
}

func TestExtractDepsFromLabels(t *testing.T) {
	t.Run("no deps", func(t *testing.T) {
		deps := extractDepsFromLabels([]string{"bug", "enhancement"})
		assert.Empty(t, deps)
	})

	t.Run("with deps", func(t *testing.T) {
		deps := extractDepsFromLabels([]string{"depends:07-scheduling", "bug", "depends:08-permissions"})
		assert.Equal(t, []string{"07-scheduling", "08-permissions"}, deps)
	})
}

func TestFormatIssuePlan_WithTaskHeaders(t *testing.T) {
	issue := GitHubIssue{
		Number: 7,
		Title:  "Professional Scheduling",
		Body:   "Context here\n\n### Task 1: Setup\n- [ ] do stuff\n\n### Task 2: Implement\n- [ ] more stuff",
		Labels: []string{"ralphex"},
	}

	plan := formatIssuePlan(issue)
	assert.Contains(t, plan, "# Professional Scheduling")
	assert.Contains(t, plan, "Issue: #7")
	assert.Contains(t, plan, "### Task 1: Setup")
	assert.NotContains(t, plan, "### Task 1: Investigate") // should NOT wrap
}

func TestFormatIssuePlan_WithoutTaskHeaders(t *testing.T) {
	issue := GitHubIssue{
		Number: 3,
		Title:  "Fix login bug",
		Body:   "Users can't login when password has special chars",
		Labels: []string{"bug"},
	}

	plan := formatIssuePlan(issue)
	assert.Contains(t, plan, "# Fix login bug")
	assert.Contains(t, plan, "Issue: #3")
	assert.Contains(t, plan, "### Task 1: Investigate and implement")
	assert.Contains(t, plan, "- [ ] Read the issue description")
}

func TestFormatIssuePlan_WithDependencyLabels(t *testing.T) {
	issue := GitHubIssue{
		Number: 11,
		Title:  "Appointment without workspace",
		Body:   "### Task 1: Refactor\n- [ ] change stuff",
		Labels: []string{"ralphex", "depends:07-scheduling"},
	}

	plan := formatIssuePlan(issue)
	assert.Contains(t, plan, "depends_on:")
	assert.Contains(t, plan, "  - 07-scheduling")
}

func TestIssueToPlanFile(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")

	issue := GitHubIssue{
		Number: 5,
		Title:  "Add payment flow",
		Body:   "Implement stripe integration",
	}

	path, err := IssueToPlanFile(issue, plansDir)
	require.NoError(t, err)
	assert.FileExists(t, path)
	assert.Equal(t, filepath.Join(plansDir, "05-add-payment-flow.md"), path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Issue: #5")
}

func TestIssueToPlanFile_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	existing := "manually refined plan"
	path := filepath.Join(plansDir, "05-add-payment-flow.md")
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o644))

	issue := GitHubIssue{Number: 5, Title: "Add payment flow", Body: "new body"}
	resultPath, err := IssueToPlanFile(issue, plansDir)
	require.NoError(t, err)

	content, err := os.ReadFile(resultPath)
	require.NoError(t, err)
	assert.Equal(t, existing, string(content))
}

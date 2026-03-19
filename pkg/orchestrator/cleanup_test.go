package orchestrator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testLogger struct {
	messages []string
}

func (l *testLogger) Printf(format string, args ...any) {
	l.messages = append(l.messages, fmt.Sprintf(format, args...))
}

func TestCleanOldFiles(t *testing.T) {
	dir := t.TempDir()
	log := &testLogger{}

	// create old file
	oldFile := filepath.Join(dir, "old.log")
	require.NoError(t, os.WriteFile(oldFile, []byte("old"), 0o644))
	oldTime := time.Now().AddDate(0, 0, -10)
	require.NoError(t, os.Chtimes(oldFile, oldTime, oldTime))

	// create new file
	newFile := filepath.Join(dir, "new.log")
	require.NoError(t, os.WriteFile(newFile, []byte("new"), 0o644))

	cutoff := time.Now().AddDate(0, 0, -7)
	cleanOldFiles(dir, cutoff, log)

	assert.NoFileExists(t, oldFile)
	assert.FileExists(t, newFile)
	assert.Len(t, log.messages, 1)
}

func TestCleanOldFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	log := &testLogger{}
	cleanOldFiles(dir, time.Now(), log)
	assert.Empty(t, log.messages)
}

func TestCleanOldFiles_NonexistentDir(t *testing.T) {
	log := &testLogger{}
	cleanOldFiles("/nonexistent/dir", time.Now(), log)
	// should not panic
}

func TestRecordProblem(t *testing.T) {
	dir := t.TempDir()

	RecordProblem(dir, "07-scheduling", errors.New("worktree already exists"), "stale worktree from crashed run")
	RecordProblem(dir, "08-permissions", errors.New("API Error: 500"), "rate limiting from parallel execution")

	content, err := os.ReadFile(filepath.Join(dir, ".ralphex", "problems.md"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "07-scheduling")
	assert.Contains(t, s, "worktree already exists")
	assert.Contains(t, s, "stale worktree from crashed run")
	assert.Contains(t, s, "08-permissions")
	assert.Contains(t, s, "API Error: 500")
}

func TestRecordProblem_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "deep", "nested")

	RecordProblem(subdir, "test", errors.New("fail"), "")

	assert.FileExists(t, filepath.Join(subdir, ".ralphex", "problems.md"))
}

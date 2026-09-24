package fsext

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldExcludeFileHonorsSalameyIgnore(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	ignoredFile := filepath.Join(rootDir, "secret.log")
	keptFile := filepath.Join(rootDir, "notes.txt")

	require.NoError(t, os.WriteFile(filepath.Join(rootDir, ".salameyignore"), []byte("*.log\n"), 0o644))
	require.NoError(t, os.WriteFile(ignoredFile, []byte("secret"), 0o644))
	require.NoError(t, os.WriteFile(keptFile, []byte("notes"), 0o644))

	require.True(t, ShouldExcludeFile(rootDir, ignoredFile))
	require.False(t, ShouldExcludeFile(rootDir, keptFile))
}

func TestGlobGitignoreAwareHonorsSalameyIgnore(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	goodFile := filepath.Join(rootDir, "good.txt")
	ignoredFile := filepath.Join(rootDir, "bad.tmp")

	require.NoError(t, os.WriteFile(filepath.Join(rootDir, ".salameyignore"), []byte("*.tmp\n"), 0o644))
	require.NoError(t, os.WriteFile(goodFile, []byte("good"), 0o644))
	require.NoError(t, os.WriteFile(ignoredFile, []byte("bad"), 0o644))

	matches, truncated, err := GlobGitignoreAware("*.tmp", rootDir, 0)
	require.NoError(t, err)
	require.False(t, truncated)
	require.Empty(t, matches)

	matches, truncated, err = GlobGitignoreAware("*.txt", rootDir, 0)
	require.NoError(t, err)
	require.False(t, truncated)
	require.Equal(t, []string{goodFile}, matches)
}

func TestShouldExcludeFileInsideSalameyDirectory(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	dataDir := filepath.Join(rootDir, ".salamey")
	stateFile := filepath.Join(dataDir, "state.json")

	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.WriteFile(stateFile, []byte("{}"), 0o644))

	require.True(t, ShouldExcludeFile(rootDir, stateFile))
	require.True(t, SkipHidden(stateFile))
}

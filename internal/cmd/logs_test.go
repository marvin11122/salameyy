package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindLogFileFallsBackToLegacyProject(t *testing.T) {
	t.Setenv("SALAMEY_APP_IDENTITY", "salamey")
	root := t.TempDir()
	dataDir := filepath.Join(root, ".salamey")
	legacyLog := filepath.Join(root, ".crush", "logs", "crush.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyLog), 0o755))
	require.NoError(t, os.WriteFile(legacyLog, []byte("legacy\n"), 0o644))

	found := findLogFile(dataDir, root, filepath.Join(root, "global", "salamey.json"))
	require.Equal(t, legacyLog, found)
}

func TestFindLogFilePrefersSalameyProject(t *testing.T) {
	t.Setenv("SALAMEY_APP_IDENTITY", "salamey")
	root := t.TempDir()
	dataDir := filepath.Join(root, ".salamey")
	activeLog := filepath.Join(dataDir, "logs", "salamey.log")
	legacyLog := filepath.Join(root, ".crush", "logs", "crush.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(activeLog), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyLog), 0o755))
	require.NoError(t, os.WriteFile(activeLog, []byte("active\n"), 0o644))
	require.NoError(t, os.WriteFile(legacyLog, []byte("legacy\n"), 0o644))

	found := findLogFile(dataDir, root, filepath.Join(root, "global", "salamey.json"))
	require.Equal(t, activeLog, found)
}

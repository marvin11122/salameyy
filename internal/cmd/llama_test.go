package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
)

func TestMergedLlamaOptionsUsesPortOverride(t *testing.T) {
	t.Parallel()
	command := &cobra.Command{}
	command.Flags().String("host", "", "")
	command.Flags().Int("port", 0, "")
	command.Flags().String("base-url", "", "")
	require.NoError(t, command.Flags().Set("port", "9090"))
	opts, err := mergedLlamaOptions(command, &config.LlamaCPPOptions{BaseURL: "http://127.0.0.1:8080/v1/", Host: "127.0.0.1", Port: 8080})
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:9090/v1/", opts.BaseURL)
}

func TestDiscoverGGUFPathsNonRecursive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	require.NoError(t, os.Mkdir(nested, 0o755))
	top := filepath.Join(root, "top.gguf")
	nestedFile := filepath.Join(nested, "nested.gguf")
	require.NoError(t, os.WriteFile(top, []byte("top"), 0o600))
	require.NoError(t, os.WriteFile(nestedFile, []byte("nested"), 0o600))
	paths, err := discoverGGUFPaths(root, false)
	require.NoError(t, err)
	require.Equal(t, []string{top}, paths)
}

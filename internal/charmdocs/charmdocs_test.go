package charmdocs

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDiscoversExpectedDocs(t *testing.T) {
	t.Parallel()

	catalog := loadTestCatalog(t)
	slugs := catalog.Slugs()

	require.Contains(t, slugs, "bubbletea")
	require.Contains(t, slugs, "x")
}

func TestLoadFindsDocsFromNestedDirectory(t *testing.T) {
	t.Parallel()

	root := testRoot(t)
	catalog, err := Load(filepath.Join(root, "internal", "cmd"))
	require.NoError(t, err)
	require.Equal(t, root, catalog.Root())
}

func TestPreviewAndRenderAreNonEmpty(t *testing.T) {
	t.Parallel()

	catalog := loadTestCatalog(t)

	preview, err := catalog.Preview("bubbletea", 72, 8)
	require.NoError(t, err)
	require.NotEmpty(t, preview)

	rendered, err := catalog.Render("x", 72)
	require.NoError(t, err)
	require.NotEmpty(t, rendered)
}

func TestSlugsAreSorted(t *testing.T) {
	t.Parallel()

	catalog := loadTestCatalog(t)
	slugs := catalog.Slugs()

	require.Len(t, slugs, len(catalog.docs))
	sorted := slices.Clone(slugs)
	slices.Sort(sorted)
	require.Equal(t, sorted, slugs)
}

func TestLoadErrorsWhenDocsDirectoryIsMissing(t *testing.T) {
	t.Parallel()

	_, err := Load(t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, "could not find docs/charm")
}

func loadTestCatalog(t *testing.T) Catalog {
	t.Helper()

	catalog, err := Load(testRoot(t))
	require.NoError(t, err)

	return catalog
}

func testRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

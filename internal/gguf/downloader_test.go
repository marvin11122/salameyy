package gguf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHuggingFaceResolveURLUsesSafeRelativePath(t *testing.T) {
	t.Parallel()

	endpoint, err := huggingFaceResolveURL("owner/repo", "refs/pr/1", "q4/model.gguf")
	require.NoError(t, err)
	require.Equal(t, "https://huggingface.co/owner/repo/resolve/refs/pr/1/q4/model.gguf", endpoint)
}

func TestDownloadFilenameValidationRejectsTraversal(t *testing.T) {
	t.Parallel()

	_, err := safeDownloadName("../model.gguf")
	require.Error(t, err)
	_, err = huggingFaceResolveURL("owner/repo", "main", "folder/../model.gguf")
	require.Error(t, err)
}

func TestParseModelID(t *testing.T) {
	t.Parallel()

	owner, repo, revision := ParseModelID("owner/repo@tag")
	require.Equal(t, "owner", owner)
	require.Equal(t, "repo", repo)
	require.Equal(t, "tag", revision)
}

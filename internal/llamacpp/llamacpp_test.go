package llamacpp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildArgsUsesCurrentFlashAttentionFlag(t *testing.T) {
	t.Parallel()

	flash := true
	noMMap := true
	args := buildArgs(StartOptions{ModelPath: "/models/oss.gguf", Host: "127.0.0.1", Port: 8081, ContextSize: 32768, GPULayers: 99, Threads: 8, Parallel: 2, FlashAttention: &flash, NoMMap: &noMMap, Args: []string{"--jinja"}})
	require.Equal(t, []string{"-m", "/models/oss.gguf", "--host", "127.0.0.1", "--port", "8081", "-c", "32768", "-ngl", "99", "-t", "8", "-np", "2", "-fa", "--no-mmap", "--jinja"}, args)
}

func TestQueryModelsUsesContextAndDecodesMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/v1/models", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"data":[{"id":"local-7b","name":"Local 7B","meta":{"n_ctx_train":65536}}]}`)
	}))
	defer server.Close()

	models, err := QueryModels(context.Background(), server.URL)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "local-7b", models[0].ID)
	require.EqualValues(t, 65536, models[0].ContextWindow)
}

func TestSelectDefaultModelsPrefersCodingModels(t *testing.T) {
	t.Parallel()

	large, small := SelectDefaultModels([]ModelInfo{
		{ID: "gpt-oss-20b-IQ4_NL.gguf", Name: "GPT OSS 20B", ContextWindow: 131072},
		{ID: "qwen2.5-coder-7b-instruct-q4_k_m.gguf", Name: "Qwen 2.5 Coder 7B", ContextWindow: 32768},
		{ID: "qwen2.5-coder-3b-instruct-q4_k_m.gguf", Name: "Qwen 2.5 Coder 3B", ContextWindow: 32768},
		{ID: "nomic-embed-text-v1.5-q8.gguf", Name: "Nomic Embed", ContextWindow: 8192},
	})
	require.Equal(t, "qwen2.5-coder-7b-instruct-q4_k_m.gguf", large.ID)
	require.Equal(t, "qwen2.5-coder-3b-instruct-q4_k_m.gguf", small.ID)
}

func TestQueryModelsHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := QueryModels(ctx, "http://127.0.0.1:1")
	require.ErrorIs(t, err, context.Canceled)
}

func TestWaitUntilReadyReturnsContextError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := WaitUntilReady(ctx, "http://127.0.0.1:1", time.Millisecond)
	require.ErrorIs(t, err, context.Canceled)
}

func TestManagerStateRoundTrip(t *testing.T) {
	t.Parallel()

	manager := NewManager(t.TempDir())
	state := &State{PID: 4242, Command: "llama-server", BaseURL: DefaultBaseURL, ModelPath: "/models/local.gguf", LogPath: filepath.Join(manager.dataDir, "logs", "llama.cpp.log"), StartedAt: time.Now().UTC().Round(time.Second)}
	require.NoError(t, manager.saveState(state))
	loaded, err := manager.LoadState()
	require.NoError(t, err)
	require.Equal(t, state.PID, loaded.PID)
	require.Equal(t, state.Command, loaded.Command)
	require.Equal(t, state.BaseURL, loaded.BaseURL)
}

func TestSignalProcessRejectsUnsafePID(t *testing.T) {
	t.Parallel()

	require.False(t, processRunning(0))
	require.False(t, processRunning(1))
	require.Error(t, signalProcess(os.Getpid(), false))
}

func TestMergeEnvOverridesSafely(t *testing.T) {
	t.Parallel()

	merged := mergeEnv([]string{"A=1", "B=2", "INVALID=old", "A=3"}, map[string]string{"B": "new", "C": "value", "BAD\nKEY": "ignored"})
	require.Contains(t, merged, "A=3")
	require.Contains(t, merged, "B=new")
	require.Contains(t, merged, "C=value")
	require.Contains(t, merged, "INVALID=old")
	require.NotContains(t, merged, "BAD\nKEY=ignored")
}

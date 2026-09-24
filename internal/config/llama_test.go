package config

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyLlamaCPPDefaults(t *testing.T) {
	t.Parallel()
	cfg := &Config{Options: &Options{}}
	applyLlamaCPPDefaults(cfg)
	require.Equal(t, "llama-server", cfg.Options.LlamaCPP.Command)
	require.Equal(t, "127.0.0.1", cfg.Options.LlamaCPP.Host)
	require.Equal(t, 8080, cfg.Options.LlamaCPP.Port)
	require.Equal(t, 20, cfg.Options.LlamaCPP.HealthcheckTimeout)
	require.Equal(t, "http://127.0.0.1:8080/v1/", cfg.Options.LlamaCPP.BaseURL)
}

func TestDiscoverLocalLlamaProvider(t *testing.T) {
	t.Setenv("SALAMEY_APP_IDENTITY", "salamey")
	t.Setenv("SALAMEY_LLAMA_BASE_URL", "")
	t.Setenv("CRUSH_LLAMA_BASE_URL", "")
	t.Setenv("LLAMA_BASE_URL", "")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/v1/models", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":[{"id":"coder-7b","name":"Coder 7B","meta":{"n_ctx_train":65536}}]}`))
	}))
	defer server.Close()

	cfg := &Config{Options: &Options{LlamaCPP: &LlamaCPPOptions{BaseURL: server.URL}}}
	provider, err := DiscoverLocalLlamaProvider(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, "llamacpp", string(provider.ID))
	require.Equal(t, "coder-7b", provider.DefaultLargeModelID)
	require.Equal(t, int64(65536), provider.Models[0].ContextWindow)
}

func TestDiscoverLocalLlamaProviderRespectsDisabled(t *testing.T) {
	t.Parallel()
	cfg := &Config{Options: &Options{LlamaCPP: &LlamaCPPOptions{Disabled: true}}}
	_, err := DiscoverLocalLlamaProvider(context.Background(), cfg)
	require.Error(t, err)
}

func TestLocalLlamaBaseURLEnvironmentPrecedence(t *testing.T) {
	t.Setenv("SALAMEY_APP_IDENTITY", "salamey")
	t.Setenv("SALAMEY_LLAMA_BASE_URL", "http://canonical.example:9000/v1/")
	t.Setenv("LLAMA_BASE_URL", "http://bare.example:9001/v1/")
	cfg := &Config{Options: &Options{LlamaCPP: &LlamaCPPOptions{BaseURL: "http://config.example:9002/v1/"}}}
	require.Equal(t, "http://canonical.example:9000/v1/", LocalLlamaBaseURL(cfg))
}

func TestLlamaCPPOptionsJSONRoundTrip(t *testing.T) {
	t.Parallel()
	input := LlamaCPPOptions{Command: "llama-server", BaseURL: "http://127.0.0.1:8080/v1/", Port: 8080, ContextSize: 16384, FlashAttention: boolPointer(true)}
	data, err := json.Marshal(input)
	require.NoError(t, err)
	var output LlamaCPPOptions
	require.NoError(t, json.Unmarshal(data, &output))
	require.Equal(t, input.Command, output.Command)
	require.Equal(t, input.BaseURL, output.BaseURL)
	require.NotNil(t, output.FlashAttention)
	require.True(t, *output.FlashAttention)
}

func boolPointer(value bool) *bool {
	return &value
}

package config

import (
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/stretchr/testify/require"
	"github.com/wiesnermarven-hue/salameyy/internal/csync"
	"github.com/wiesnermarven-hue/salameyy/internal/env"
)

func TestConfigureProvidersAllowsKeylessLlama(t *testing.T) {
	t.Parallel()
	environment := env.New()
	cfg := &Config{
		Options:   &Options{DisableDefaultProviders: true},
		Providers: csync.NewMap[string, ProviderConfig](),
	}
	provider := catwalk.Provider{
		ID:                  "llamacpp",
		Name:                "llama.cpp",
		APIEndpoint:         "http://127.0.0.1:8080/v1/",
		Type:                catwalk.TypeOpenAICompat,
		DefaultLargeModelID: "coder",
		DefaultSmallModelID: "coder",
		Models:              []catwalk.Model{{ID: "coder", ContextWindow: 32768}},
	}
	err := cfg.configureProviders(environment, NewShellVariableResolver(environment), []catwalk.Provider{provider})
	require.NoError(t, err)
	configured, ok := cfg.Providers.Get("llamacpp")
	require.True(t, ok)
	require.Empty(t, configured.APIKey)
	require.Equal(t, "http://127.0.0.1:8080/v1/", configured.BaseURL)
	require.Equal(t, catwalk.TypeOpenAICompat, configured.Type)
}

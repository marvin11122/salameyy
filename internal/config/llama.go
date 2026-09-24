package config

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/wiesnermarven-hue/salameyy/internal/env"
	"github.com/wiesnermarven-hue/salameyy/internal/llamacpp"
)

const localLlamaProviderID = "llamacpp"

func LoadOptions(workingDir, dataDir string, debug bool) (*Config, error) {
	cfg, err := loadFromConfigPaths(lookupConfigs(workingDir))
	if err != nil {
		return nil, fmt.Errorf("failed to load config from paths: %w", err)
	}
	cfg.dataConfigDir = GlobalConfigData()
	cfg.setDefaults(workingDir, dataDir)
	if debug {
		cfg.Options.Debug = true
	}
	cfg.resolver = NewShellVariableResolver(env.New())
	return cfg, nil
}

func applyLlamaCPPDefaults(c *Config) {
	if c == nil || c.Options == nil {
		return
	}
	if c.Options.LlamaCPP == nil {
		c.Options.LlamaCPP = &LlamaCPPOptions{}
	}
	opts := c.Options.LlamaCPP
	opts.Command = strings.TrimSpace(opts.Command)
	if opts.Command == "" {
		opts.Command = "llama-server"
	}
	opts.Host = strings.TrimSpace(opts.Host)
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.Port == 0 {
		opts.Port = 8080
	}
	if opts.HealthcheckTimeout <= 0 {
		opts.HealthcheckTimeout = 20
	}
	if strings.TrimSpace(opts.BaseURL) == "" {
		opts.BaseURL = llamacpp.NormalizeBaseURL("", opts.Host, opts.Port)
	}
}

func resolveLlamaValue(cfg *Config, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || cfg == nil || cfg.resolver == nil {
		return value
	}
	resolved, err := cfg.Resolve(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(resolved)
}

func LocalLlamaBaseURL(cfg *Config) string {
	if cfg == nil || cfg.Options == nil || cfg.Options.LlamaCPP == nil {
		return llamacpp.DefaultBaseURL
	}
	opts := cfg.Options.LlamaCPP
	baseURL := resolveLlamaValue(cfg, opts.BaseURL)
	host := resolveLlamaValue(cfg, opts.Host)
	port := opts.Port
	if envURL, ok := lookupCompatNonEmptyEnv("LLAMA_BASE_URL"); ok {
		baseURL = envURL
	} else if envURL := strings.TrimSpace(os.Getenv("LLAMA_BASE_URL")); envURL != "" {
		baseURL = envURL
	}
	return llamacpp.NormalizeBaseURL(baseURL, host, port)
}

func localLlamaDisabled(cfg *Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.Options != nil && cfg.Options.LlamaCPP != nil && cfg.Options.LlamaCPP.Disabled {
		return true
	}
	if cfg.Providers != nil {
		if provider, ok := cfg.Providers.Get(localLlamaProviderID); ok && provider.Disable {
			return true
		}
	}
	return false
}

func DiscoverLocalLlamaProvider(ctx context.Context, cfg *Config) (catwalk.Provider, error) {
	if localLlamaDisabled(cfg) {
		return catwalk.Provider{}, fmt.Errorf("local llama.cpp runtime is disabled")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return llamacpp.ProviderFromURL(ctx, LocalLlamaBaseURL(cfg))
}

func shouldAutoStartLocalLlama(cfg *Config) bool {
	if localLlamaDisabled(cfg) || cfg.Options == nil || cfg.Options.LlamaCPP == nil {
		return false
	}
	opts := cfg.Options.LlamaCPP
	if !opts.AutoStart || strings.TrimSpace(opts.Model) == "" {
		return false
	}
	parsed, err := url.Parse(LocalLlamaBaseURL(cfg))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func StartLocalLlama(ctx context.Context, cfg *Config) error {
	if localLlamaDisabled(cfg) {
		return fmt.Errorf("local llama.cpp runtime is disabled")
	}
	if cfg == nil || cfg.Options == nil || cfg.Options.LlamaCPP == nil {
		return fmt.Errorf("local llama.cpp configuration is missing")
	}
	opts := cfg.Options.LlamaCPP
	modelPath := resolveLlamaValue(cfg, opts.Model)
	if modelPath == "" {
		return fmt.Errorf("llama.cpp model path is empty")
	}
	if !filepath.IsAbs(modelPath) {
		base := cfg.WorkingDir()
		if base == "" {
			var err error
			base, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		modelPath = filepath.Join(base, modelPath)
	}
	modelPath, err := filepath.Abs(modelPath)
	if err != nil {
		return err
	}
	workingDir := resolveLlamaValue(cfg, opts.WorkingDir)
	if workingDir != "" && !filepath.IsAbs(workingDir) {
		workingDir = filepath.Join(cfg.WorkingDir(), workingDir)
	}
	if workingDir != "" {
		workingDir, err = filepath.Abs(workingDir)
		if err != nil {
			return err
		}
	}
	timeout := time.Duration(opts.HealthcheckTimeout) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	manager := llamacpp.NewManager(cfg.Options.DataDirectory)
	_, err = manager.Start(ctx, llamacpp.StartOptions{
		Command:            resolveLlamaValue(cfg, opts.Command),
		Args:               append([]string(nil), opts.Args...),
		Env:                opts.Env,
		BaseURL:            LocalLlamaBaseURL(cfg),
		ModelPath:          filepath.Clean(modelPath),
		Host:               resolveLlamaValue(cfg, opts.Host),
		Port:               opts.Port,
		WorkingDir:         workingDir,
		HealthcheckTimeout: timeout,
		ContextSize:        opts.ContextSize,
		GPULayers:          opts.GPULayers,
		Threads:            opts.Threads,
		Parallel:           opts.Parallel,
		FlashAttention:     opts.FlashAttention,
		NoMMap:             opts.NoMMap,
	})
	return err
}

func appendLocalLlamaProvider(cfg *Config, providers []catwalk.Provider) ([]catwalk.Provider, error) {
	if localLlamaDisabled(cfg) {
		return providers, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	provider, err := DiscoverLocalLlamaProvider(ctx, cfg)
	if err != nil {
		return providers, err
	}
	for i, existing := range providers {
		if string(existing.ID) == localLlamaProviderID {
			providers[i] = provider
			return providers, nil
		}
	}
	return append(providers, provider), nil
}

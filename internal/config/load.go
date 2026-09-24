package config

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	powernapConfig "github.com/charmbracelet/x/powernap/pkg/config"
	"github.com/qjebbs/go-jsons"
	"github.com/wiesnermarven-hue/salameyy/internal/agent/hyper"
	"github.com/wiesnermarven-hue/salameyy/internal/csync"
	"github.com/wiesnermarven-hue/salameyy/internal/env"
	"github.com/wiesnermarven-hue/salameyy/internal/fsext"
	"github.com/wiesnermarven-hue/salameyy/internal/home"
	"github.com/wiesnermarven-hue/salameyy/internal/log"
)

const defaultCatwalkURL = "https://catwalk.charm.sh"

// Load loads the configuration from the default paths.
func Load(workingDir, dataDir string, debug bool) (*Config, error) {
	configPaths := lookupConfigs(workingDir)

	cfg, err := loadFromConfigPaths(configPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from paths %v: %w", configPaths, err)
	}

	cfg.dataConfigDir = GlobalConfigData()

	cfg.setDefaults(workingDir, dataDir)

	if debug {
		cfg.Options.Debug = true
	}

	// Setup logs
	log.Setup(
		filepath.Join(cfg.Options.DataDirectory, "logs", RuntimeLogFileName()),
		cfg.Options.Debug,
	)

	if !isInsideWorktree() {
		const depth = 2
		const items = 100
		slog.Warn("No git repository detected in working directory, will limit file walk operations", "depth", depth, "items", items)
		assignIfNil(&cfg.Tools.Ls.MaxDepth, depth)
		assignIfNil(&cfg.Tools.Ls.MaxItems, items)
		assignIfNil(&cfg.Options.TUI.Completions.MaxDepth, depth)
		assignIfNil(&cfg.Options.TUI.Completions.MaxItems, items)
	}

	if isAppleTerminal() {
		slog.Warn("Detected Apple Terminal, enabling transparent mode")
		assignIfNil(&cfg.Options.TUI.Transparent, true)
	}

	env := env.New()
	valueResolver := NewShellVariableResolver(env)
	cfg.resolver = valueResolver

	providers, err := Providers(cfg)
	if err != nil && len(providers) > 0 {
		slog.Warn("Proceeding with partial provider metadata", "error", err)
	}
	if shouldAutoStartLocalLlama(cfg) {
		startCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		startErr := StartLocalLlama(startCtx, cfg)
		cancel()
		if startErr != nil {
			slog.Warn("Failed to auto-start llama.cpp", "error", startErr)
		}
	}
	providers, localErr := appendLocalLlamaProvider(cfg, providers)
	if localErr != nil {
		if err != nil && len(providers) == 0 {
			return nil, err
		}
		slog.Debug("Local llama.cpp runtime is not available", "error", localErr)
	}
	cfg.knownProviders = providers

	if err := cfg.configureProviders(env, valueResolver, cfg.knownProviders); err != nil {
		return nil, fmt.Errorf("failed to configure providers: %w", err)
	}

	if !cfg.IsConfigured() {
		slog.Warn("No providers configured")
		return cfg, nil
	}

	if err := cfg.configureSelectedModels(cfg.knownProviders); err != nil {
		return nil, fmt.Errorf("failed to configure selected models: %w", err)
	}
	cfg.SetupAgents()
	return cfg, nil
}

func PushPopAppEnv() func() {
	foundSet := make(map[string]struct{})
	for _, ev := range os.Environ() {
		pair := strings.SplitN(ev, "=", 2)
		if len(pair) != 2 {
			continue
		}
		switch {
		case strings.HasPrefix(pair[0], "CRUSH_"):
			foundSet[strings.TrimPrefix(pair[0], "CRUSH_")] = struct{}{}
		case strings.HasPrefix(pair[0], "SALAMEY_"):
			foundSet[strings.TrimPrefix(pair[0], "SALAMEY_")] = struct{}{}
		}
	}

	found := slices.Collect(maps.Keys(foundSet))
	slices.Sort(found)

	type backup struct {
		value string
		ok    bool
	}
	backups := make(map[string]backup, len(found))
	for _, ev := range found {
		value, ok := os.LookupEnv(ev)
		backups[ev] = backup{value: value, ok: ok}
	}

	for _, ev := range found {
		if RuntimeAppName() == appAliasName {
			if val, ok := os.LookupEnv("CRUSH_" + ev); ok {
				os.Setenv(ev, val)
				continue
			}
			if val, ok := os.LookupEnv("SALAMEY_" + ev); ok {
				os.Setenv(ev, val)
			}
		} else {
			if val, ok := os.LookupEnv("SALAMEY_" + ev); ok {
				os.Setenv(ev, val)
				continue
			}
			if val, ok := os.LookupEnv("CRUSH_" + ev); ok {
				os.Setenv(ev, val)
			}
		}
	}

	restore := func() {
		for k, v := range backups {
			if v.ok {
				os.Setenv(k, v.value)
			} else {
				os.Unsetenv(k)
			}
		}
	}
	return restore
}

func (c *Config) configureProviders(env env.Env, resolver VariableResolver, knownProviders []catwalk.Provider) error {
	knownProviderNames := make(map[string]bool)
	restore := PushPopAppEnv()
	defer restore()

	if c.Options.DisableDefaultProviders {
		filtered := make([]catwalk.Provider, 0, 1)
		for _, provider := range knownProviders {
			if string(provider.ID) == localLlamaProviderID {
				filtered = append(filtered, provider)
			}
		}
		knownProviders = filtered
	}

	for _, p := range knownProviders {
		knownProviderNames[string(p.ID)] = true
		config, configExists := c.Providers.Get(string(p.ID))
		// if the user configured a known provider we need to allow it to override a couple of parameters
		if configExists {
			if config.BaseURL != "" {
				p.APIEndpoint = config.BaseURL
			}
			if config.APIKey != "" {
				p.APIKey = config.APIKey
			}
			if len(config.Models) > 0 {
				models := []catwalk.Model{}
				seen := make(map[string]bool)

				for _, model := range config.Models {
					if seen[model.ID] {
						continue
					}
					seen[model.ID] = true
					if model.Name == "" {
						model.Name = model.ID
					}
					models = append(models, model)
				}
				for _, model := range p.Models {
					if seen[model.ID] {
						continue
					}
					seen[model.ID] = true
					if model.Name == "" {
						model.Name = model.ID
					}
					models = append(models, model)
				}

				p.Models = models
			}
		}

		headers := map[string]string{}
		if len(p.DefaultHeaders) > 0 {
			maps.Copy(headers, p.DefaultHeaders)
		}
		if len(config.ExtraHeaders) > 0 {
			maps.Copy(headers, config.ExtraHeaders)
		}
		for k, v := range headers {
			resolved, err := resolver.ResolveValue(v)
			if err != nil {
				slog.Error("Could not resolve provider header", "err", err.Error())
				continue
			}
			headers[k] = resolved
		}
		prepared := ProviderConfig{
			ID:                 string(p.ID),
			Name:               p.Name,
			BaseURL:            p.APIEndpoint,
			APIKey:             p.APIKey,
			APIKeyTemplate:     p.APIKey, // Store original template for re-resolution
			OAuthToken:         config.OAuthToken,
			Type:               p.Type,
			Disable:            config.Disable,
			SystemPromptPrefix: config.SystemPromptPrefix,
			ExtraHeaders:       headers,
			ExtraBody:          config.ExtraBody,
			ExtraParams:        make(map[string]string),
			Models:             p.Models,
		}

		switch {
		case p.ID == catwalk.InferenceProviderAnthropic && config.OAuthToken != nil:
			// Claude Code subscription is not supported anymore. Remove to show onboarding.
			c.RemoveConfigField("providers.anthropic")
			c.Providers.Del(string(p.ID))
			continue
		case p.ID == catwalk.InferenceProviderCopilot && config.OAuthToken != nil:
			prepared.SetupGitHubCopilot()
		}

		switch p.ID {
		// Handle specific providers that require additional configuration
		case catwalk.InferenceProviderVertexAI:
			var (
				project  = env.Get("VERTEXAI_PROJECT")
				location = env.Get("VERTEXAI_LOCATION")
			)
			if project == "" || location == "" {
				if configExists {
					slog.Warn("Skipping Vertex AI provider due to missing credentials")
					c.Providers.Del(string(p.ID))
				}
				continue
			}
			prepared.ExtraParams["project"] = project
			prepared.ExtraParams["location"] = location
		case catwalk.InferenceProviderAzure:
			endpoint, err := resolver.ResolveValue(p.APIEndpoint)
			if err != nil || endpoint == "" {
				if configExists {
					slog.Warn("Skipping Azure provider due to missing API endpoint", "provider", p.ID, "error", err)
					c.Providers.Del(string(p.ID))
				}
				continue
			}
			prepared.BaseURL = endpoint
			prepared.ExtraParams["apiVersion"] = env.Get("AZURE_OPENAI_API_VERSION")
		case catwalk.InferenceProviderBedrock:
			if !hasAWSCredentials(env) {
				if configExists {
					slog.Warn("Skipping Bedrock provider due to missing AWS credentials")
					c.Providers.Del(string(p.ID))
				}
				continue
			}
			prepared.ExtraParams["region"] = env.Get("AWS_REGION")
			if prepared.ExtraParams["region"] == "" {
				prepared.ExtraParams["region"] = env.Get("AWS_DEFAULT_REGION")
			}
			for _, model := range p.Models {
				if !strings.HasPrefix(model.ID, "anthropic.") {
					return fmt.Errorf("bedrock provider only supports anthropic models for now, found: %s", model.ID)
				}
			}
		default:
			v, err := resolver.ResolveValue(p.APIKey)
			if (v == "" || err != nil) && string(p.ID) != localLlamaProviderID {
				if configExists {
					slog.Warn("Skipping provider due to missing API key", "provider", p.ID)
					c.Providers.Del(string(p.ID))
				}
				continue
			}
		}
		c.Providers.Set(string(p.ID), prepared)
	}

	// validate the custom providers
	for id, providerConfig := range c.Providers.Seq2() {
		if knownProviderNames[id] {
			continue
		}

		// Make sure the provider ID is set
		providerConfig.ID = id
		providerConfig.Name = cmp.Or(providerConfig.Name, id) // Use ID as name if not set
		// default to OpenAI if not set
		providerConfig.Type = cmp.Or(providerConfig.Type, catwalk.TypeOpenAICompat)
		if !slices.Contains(catwalk.KnownProviderTypes(), providerConfig.Type) && providerConfig.Type != hyper.Name && providerConfig.Type != "llama" {
			slog.Warn("Skipping custom provider due to unsupported provider type", "provider", id)
			c.Providers.Del(id)
			continue
		}

		if providerConfig.Disable {
			slog.Debug("Skipping custom provider due to disable flag", "provider", id)
			c.Providers.Del(id)
			continue
		}
		if providerConfig.APIKey == "" {
			slog.Warn("Provider is missing API key, this might be OK for local providers", "provider", id)
		}
		if providerConfig.BaseURL == "" {
			slog.Warn("Skipping custom provider due to missing API endpoint", "provider", id)
			c.Providers.Del(id)
			continue
		}
		if len(providerConfig.Models) == 0 {
			slog.Warn("Skipping custom provider because the provider has no models", "provider", id)
			c.Providers.Del(id)
			continue
		}
		apiKey, err := resolver.ResolveValue(providerConfig.APIKey)
		if apiKey == "" || err != nil {
			slog.Warn("Provider is missing API key, this might be OK for local providers", "provider", id)
		}
		baseURL, err := resolver.ResolveValue(providerConfig.BaseURL)
		if baseURL == "" || err != nil {
			slog.Warn("Skipping custom provider due to missing API endpoint", "provider", id, "error", err)
			c.Providers.Del(id)
			continue
		}

		for k, v := range providerConfig.ExtraHeaders {
			resolved, err := resolver.ResolveValue(v)
			if err != nil {
				slog.Error("Could not resolve provider header", "err", err.Error())
				continue
			}
			providerConfig.ExtraHeaders[k] = resolved
		}

		c.Providers.Set(id, providerConfig)
	}

	if c.Providers.Len() == 0 && c.Options.DisableDefaultProviders {
		return fmt.Errorf("default providers are disabled and there are no custom providers are configured")
	}

	return nil
}

func (c *Config) setDefaults(workingDir, dataDir string) {
	c.workingDir = workingDir
	if c.Options == nil {
		c.Options = &Options{}
	}
	if c.Options.TUI == nil {
		c.Options.TUI = &TUIOptions{}
	}
	assignIfNil(&c.Options.TUI.PhysicsFace, true)
	assignIfNil(&c.Options.TUI.ShowMetrics, true)
	switch c.Options.TUI.AnimationSpeed {
	case "", "off", "slow", "medium", "fast":
		if c.Options.TUI.AnimationSpeed == "" {
			c.Options.TUI.AnimationSpeed = "medium"
		}
	default:
		c.Options.TUI.AnimationSpeed = "medium"
	}
	applyLlamaCPPDefaults(c)
	if dataDir != "" {
		c.Options.DataDirectory = dataDir
	} else if c.Options.DataDirectory == "" {
		if path, ok := lookupClosestDataDirectory(workingDir); ok {
			c.Options.DataDirectory = path
		} else {
			c.Options.DataDirectory = filepath.Join(workingDir, RuntimeDataDirectoryName())
		}
	}
	if c.Providers == nil {
		c.Providers = csync.NewMap[string, ProviderConfig]()
	}
	if c.Models == nil {
		c.Models = make(map[SelectedModelType]SelectedModel)
	}
	if c.RecentModels == nil {
		c.RecentModels = make(map[SelectedModelType][]SelectedModel)
	}
	if c.MCP == nil {
		c.MCP = make(map[string]MCPConfig)
	}
	if c.LSP == nil {
		c.LSP = make(map[string]LSPConfig)
	}

	// Apply defaults to LSP configurations
	c.applyLSPDefaults()

	// Add the default context paths if they are not already present
	c.Options.ContextPaths = append(defaultContextPaths, c.Options.ContextPaths...)
	slices.Sort(c.Options.ContextPaths)
	c.Options.ContextPaths = slices.Compact(c.Options.ContextPaths)

	// Add the default skills directories if not already present.
	for _, dir := range GlobalSkillsDirs() {
		if !slices.Contains(c.Options.SkillsPaths, dir) {
			c.Options.SkillsPaths = append(c.Options.SkillsPaths, dir)
		}
	}

	if str, ok := lookupCompatEnv("DISABLE_PROVIDER_AUTO_UPDATE"); ok {
		c.Options.DisableProviderAutoUpdate, _ = strconv.ParseBool(str)
	}

	if str, ok := lookupCompatEnv("DISABLE_DEFAULT_PROVIDERS"); ok {
		c.Options.DisableDefaultProviders, _ = strconv.ParseBool(str)
	}

	if c.Options.Attribution == nil {
		c.Options.Attribution = &Attribution{
			TrailerStyle:  TrailerStyleAssistedBy,
			GeneratedWith: true,
		}
	} else if c.Options.Attribution.TrailerStyle == "" {
		// Migrate deprecated co_authored_by or apply default
		if c.Options.Attribution.CoAuthoredBy != nil {
			if *c.Options.Attribution.CoAuthoredBy {
				c.Options.Attribution.TrailerStyle = TrailerStyleCoAuthoredBy
			} else {
				c.Options.Attribution.TrailerStyle = TrailerStyleNone
			}
		} else {
			c.Options.Attribution.TrailerStyle = TrailerStyleAssistedBy
		}
	}
	c.Options.InitializeAs = cmp.Or(c.Options.InitializeAs, defaultInitializeAs)
}

// applyLSPDefaults applies default values from powernap to LSP configurations
func (c *Config) applyLSPDefaults() {
	// Get powernap's default configuration
	configManager := powernapConfig.NewManager()
	configManager.LoadDefaults()

	// Apply defaults to each LSP configuration
	for name, cfg := range c.LSP {
		// Try to get defaults from powernap based on name or command name.
		base, ok := configManager.GetServer(name)
		if !ok {
			base, ok = configManager.GetServer(cfg.Command)
			if !ok {
				continue
			}
		}
		if cfg.Options == nil {
			cfg.Options = base.Settings
		}
		if cfg.InitOptions == nil {
			cfg.InitOptions = base.InitOptions
		}
		if len(cfg.FileTypes) == 0 {
			cfg.FileTypes = base.FileTypes
		}
		if len(cfg.RootMarkers) == 0 {
			cfg.RootMarkers = base.RootMarkers
		}
		cfg.Command = cmp.Or(cfg.Command, base.Command)
		if len(cfg.Args) == 0 {
			cfg.Args = base.Args
		}
		if len(cfg.Env) == 0 {
			cfg.Env = base.Environment
		}
		// Update the config in the map
		c.LSP[name] = cfg
	}
}

func (c *Config) defaultModelSelection(knownProviders []catwalk.Provider) (largeModel SelectedModel, smallModel SelectedModel, err error) {
	if len(knownProviders) == 0 && c.Providers.Len() == 0 {
		err = fmt.Errorf("no providers configured, please configure at least one provider")
		return largeModel, smallModel, err
	}

	// Use the first provider enabled based on the known providers order
	// if no provider found that is known use the first provider configured
	for _, p := range knownProviders {
		providerConfig, ok := c.Providers.Get(string(p.ID))
		if !ok || providerConfig.Disable {
			continue
		}
		defaultLargeModel := c.GetModel(string(p.ID), p.DefaultLargeModelID)
		if defaultLargeModel == nil {
			err = fmt.Errorf("default large model %s not found for provider %s", p.DefaultLargeModelID, p.ID)
			return largeModel, smallModel, err
		}
		largeModel = SelectedModel{
			Provider:        string(p.ID),
			Model:           defaultLargeModel.ID,
			MaxTokens:       defaultLargeModel.DefaultMaxTokens,
			ReasoningEffort: defaultLargeModel.DefaultReasoningEffort,
		}

		defaultSmallModel := c.GetModel(string(p.ID), p.DefaultSmallModelID)
		if defaultSmallModel == nil {
			err = fmt.Errorf("default small model %s not found for provider %s", p.DefaultSmallModelID, p.ID)
			return largeModel, smallModel, err
		}
		smallModel = SelectedModel{
			Provider:        string(p.ID),
			Model:           defaultSmallModel.ID,
			MaxTokens:       defaultSmallModel.DefaultMaxTokens,
			ReasoningEffort: defaultSmallModel.DefaultReasoningEffort,
		}
		return largeModel, smallModel, err
	}

	enabledProviders := c.EnabledProviders()
	slices.SortFunc(enabledProviders, func(a, b ProviderConfig) int {
		return strings.Compare(a.ID, b.ID)
	})

	if len(enabledProviders) == 0 {
		err = fmt.Errorf("no providers configured, please configure at least one provider")
		return largeModel, smallModel, err
	}

	providerConfig := enabledProviders[0]
	if len(providerConfig.Models) == 0 {
		err = fmt.Errorf("provider %s has no models configured", providerConfig.ID)
		return largeModel, smallModel, err
	}
	defaultLargeModel := c.GetModel(providerConfig.ID, providerConfig.Models[0].ID)
	largeModel = SelectedModel{
		Provider:  providerConfig.ID,
		Model:     defaultLargeModel.ID,
		MaxTokens: defaultLargeModel.DefaultMaxTokens,
	}
	defaultSmallModel := c.GetModel(providerConfig.ID, providerConfig.Models[0].ID)
	smallModel = SelectedModel{
		Provider:  providerConfig.ID,
		Model:     defaultSmallModel.ID,
		MaxTokens: defaultSmallModel.DefaultMaxTokens,
	}
	return largeModel, smallModel, err
}

func applySelectedModelSettings(model *SelectedModel, configured SelectedModel) {
	presetOnly := configured.SamplingPreset != "" && configured.Sampling == nil
	if presetOnly {
		if _, ok := SamplingPresetByID(configured.SamplingPreset); ok {
			if updated, _, err := ApplySamplingPreset(*model, configured.SamplingPreset); err == nil {
				*model = updated
			}
		}
	}
	if configured.MaxTokens > 0 {
		model.MaxTokens = configured.MaxTokens
	}
	if configured.ReasoningEffort != "" {
		model.ReasoningEffort = configured.ReasoningEffort
	}
	if !presetOnly {
		model.Think = configured.Think
	}
	if configured.Temperature != nil {
		model.Temperature = cloneFloat64Ptr(configured.Temperature)
	}
	if configured.TopP != nil {
		model.TopP = cloneFloat64Ptr(configured.TopP)
	}
	if configured.TopK != nil {
		model.TopK = cloneInt64Ptr(configured.TopK)
	}
	if configured.FrequencyPenalty != nil {
		model.FrequencyPenalty = cloneFloat64Ptr(configured.FrequencyPenalty)
	}
	if configured.PresencePenalty != nil {
		model.PresencePenalty = cloneFloat64Ptr(configured.PresencePenalty)
	}
	if configured.ProviderOptions != nil {
		model.ProviderOptions = maps.Clone(configured.ProviderOptions)
	}
	if configured.Sampling != nil {
		model.Sampling = cloneSampling(configured.Sampling)
	}
	if configured.SamplingPreset != "" {
		model.SamplingPreset = configured.SamplingPreset
	}
}

func (c *Config) configureSelectedModels(knownProviders []catwalk.Provider) error {
	defaultLarge, defaultSmall, err := c.defaultModelSelection(knownProviders)
	if err != nil {
		return fmt.Errorf("failed to select default models: %w", err)
	}
	large, small := defaultLarge, defaultSmall

	largeModelSelected, largeModelConfigured := c.Models[SelectedModelTypeLarge]
	if largeModelConfigured {
		if largeModelSelected.Model != "" {
			large.Model = largeModelSelected.Model
		}
		if largeModelSelected.Provider != "" {
			large.Provider = largeModelSelected.Provider
		}
		model := c.GetModel(large.Provider, large.Model)
		if model == nil {
			large = defaultLarge
			// override the model type to large
			err := c.UpdatePreferredModel(SelectedModelTypeLarge, large)
			if err != nil {
				return fmt.Errorf("failed to update preferred large model: %w", err)
			}
		} else {
			applySelectedModelSettings(&large, largeModelSelected)
			if largeModelSelected.MaxTokens == 0 && largeModelSelected.SamplingPreset == "" {
				large.MaxTokens = model.DefaultMaxTokens
			}
		}
	}
	smallModelSelected, smallModelConfigured := c.Models[SelectedModelTypeSmall]
	if smallModelConfigured {
		if smallModelSelected.Model != "" {
			small.Model = smallModelSelected.Model
		}
		if smallModelSelected.Provider != "" {
			small.Provider = smallModelSelected.Provider
		}

		model := c.GetModel(small.Provider, small.Model)
		if model == nil {
			small = defaultSmall
			// override the model type to small
			err := c.UpdatePreferredModel(SelectedModelTypeSmall, small)
			if err != nil {
				return fmt.Errorf("failed to update preferred small model: %w", err)
			}
		} else {
			applySelectedModelSettings(&small, smallModelSelected)
			if smallModelSelected.MaxTokens == 0 && smallModelSelected.SamplingPreset == "" {
				small.MaxTokens = model.DefaultMaxTokens
			}
		}
	}
	c.Models[SelectedModelTypeLarge] = large
	c.Models[SelectedModelTypeSmall] = small
	return nil
}

// lookupConfigs searches config files recursively from CWD up to FS root
func lookupConfigs(cwd string) []string {
	// prepend default config paths
	configPaths := append(globalConfigPaths(GlobalConfig()), globalConfigPaths(GlobalConfigData())...)

	configNames := []string{
		configFileName(RuntimeAppName()),
		"." + configFileName(RuntimeAppName()),
		configFileName(RuntimeFallbackAppName()),
		"." + configFileName(RuntimeFallbackAppName()),
	}

	foundConfigs, err := fsext.Lookup(cwd, configNames...)
	if err != nil {
		// returns at least default configs
		return configPaths
	}

	// reverse order so last config has more priority
	slices.Reverse(foundConfigs)

	return append(configPaths, foundConfigs...)
}

func lookupEnv(primary, alias string) (string, bool) {
	if value, ok := os.LookupEnv(primary); ok {
		return value, true
	}
	if alias != "" {
		if value, ok := os.LookupEnv(alias); ok {
			return value, true
		}
	}
	return "", false
}

func lookupNonEmptyEnv(primary, alias string) (string, bool) {
	if value, ok := os.LookupEnv(primary); ok && value != "" {
		return value, true
	}
	if alias != "" {
		if value, ok := os.LookupEnv(alias); ok && value != "" {
			return value, true
		}
	}
	return "", false
}

func configFileName(name string) string {
	return fmt.Sprintf("%s.json", name)
}

func globalConfigPaths(primary string) []string {
	primaryDir := filepath.Dir(primary)
	primaryName := filepath.Base(primary)
	alternateName := alternateConfigFileName(primaryName)
	paths := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	add := func(path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	if alternateName != primaryName {
		add(filepath.Join(primaryDir, alternateName))
	}

	primaryAppDir := filepath.Base(primaryDir)
	fallbackAppDir := alternateAppDirectory(primaryAppDir)
	if fallbackAppDir != primaryAppDir {
		fallbackRoot := filepath.Dir(primaryDir)
		if alternateName != primaryName {
			add(filepath.Join(fallbackRoot, fallbackAppDir, alternateName))
		}
		add(filepath.Join(fallbackRoot, fallbackAppDir, primaryName))
	}
	add(primary)
	return paths
}

func alternateAppDirectory(name string) string {
	switch name {
	case appName:
		return appAliasName
	case appAliasName:
		return appName
	default:
		return name
	}
}

func alternateConfigFileName(name string) string {
	switch name {
	case configFileName(appName):
		return configFileName(appAliasName)
	case configFileName(appAliasName):
		return configFileName(appName)
	default:
		return name
	}
}

func lookupClosestDataDirectory(workingDir string) (string, bool) {
	primaryDir, primaryOK := fsext.LookupClosest(workingDir, RuntimeDataDirectoryName())
	fallbackDir, fallbackOK := fsext.LookupClosest(workingDir, RuntimeFallbackDataDirectoryName())

	switch {
	case primaryOK && fallbackOK:
		if ancestorDistance(workingDir, filepath.Dir(primaryDir)) <= ancestorDistance(workingDir, filepath.Dir(fallbackDir)) {
			return primaryDir, true
		}
		return fallbackDir, true
	case primaryOK:
		return primaryDir, true
	case fallbackOK:
		return fallbackDir, true
	default:
		return "", false
	}
}

func ancestorDistance(startDir, ancestorDir string) int {
	current, err := filepath.Abs(startDir)
	if err != nil {
		current = startDir
	}
	target, err := filepath.Abs(ancestorDir)
	if err != nil {
		target = ancestorDir
	}

	for steps := 0; ; steps++ {
		if current == target {
			return steps
		}
		parent := filepath.Dir(current)
		if parent == current {
			return int(^uint(0) >> 1)
		}
		current = parent
	}
}

func loadFromConfigPaths(configPaths []string) (*Config, error) {
	var configs [][]byte

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to open config file %s: %w", path, err)
		}
		if len(data) == 0 {
			continue
		}
		configs = append(configs, data)
	}

	return loadFromBytes(configs)
}

func loadFromBytes(configs [][]byte) (*Config, error) {
	if len(configs) == 0 {
		return &Config{}, nil
	}

	data, err := jsons.Merge(configs)
	if err != nil {
		return nil, err
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func hasAWSCredentials(env env.Env) bool {
	if env.Get("AWS_BEARER_TOKEN_BEDROCK") != "" {
		return true
	}

	if env.Get("AWS_ACCESS_KEY_ID") != "" && env.Get("AWS_SECRET_ACCESS_KEY") != "" {
		return true
	}

	if env.Get("AWS_PROFILE") != "" || env.Get("AWS_DEFAULT_PROFILE") != "" {
		return true
	}

	if env.Get("AWS_REGION") != "" || env.Get("AWS_DEFAULT_REGION") != "" {
		return true
	}

	if env.Get("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
		env.Get("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" {
		return true
	}

	if _, err := os.Stat(filepath.Join(home.Dir(), ".aws/credentials")); err == nil && !testing.Testing() {
		return true
	}

	return false
}

// GlobalConfig returns the global configuration file path for the application.
func GlobalConfig() string {
	if configDir, ok := lookupCompatNonEmptyEnv("GLOBAL_CONFIG"); ok {
		return filepath.Join(configDir, configFileName(RuntimeAppName()))
	}
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, RuntimeAppName(), configFileName(RuntimeAppName()))
	}
	return filepath.Join(home.Dir(), ".config", RuntimeAppName(), configFileName(RuntimeAppName()))
}

// GlobalConfigData returns the path to the main data directory for the application.
// this config is used when the app overrides configurations instead of updating the global config.
func GlobalConfigData() string {
	if dataDir, ok := lookupCompatNonEmptyEnv("GLOBAL_DATA"); ok {
		return filepath.Join(dataDir, configFileName(RuntimeAppName()))
	}
	if xdgDataHome := os.Getenv("XDG_DATA_HOME"); xdgDataHome != "" {
		return filepath.Join(xdgDataHome, RuntimeAppName(), configFileName(RuntimeAppName()))
	}
	if runtime.GOOS == "windows" {
		localAppData := cmp.Or(
			os.Getenv("LOCALAPPDATA"),
			filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local"),
		)
		return filepath.Join(localAppData, RuntimeAppName(), configFileName(RuntimeAppName()))
	}
	return filepath.Join(home.Dir(), ".local", "share", RuntimeAppName(), configFileName(RuntimeAppName()))
}

func assignIfNil[T any](ptr **T, val T) {
	if *ptr == nil {
		*ptr = &val
	}
}

func isInsideWorktree() bool {
	bts, err := exec.CommandContext(
		context.Background(),
		"git", "rev-parse",
		"--is-inside-work-tree",
	).CombinedOutput()
	return err == nil && strings.TrimSpace(string(bts)) == "true"
}

// GlobalSkillsDirs returns the default directories for Agent Skills.
// Skills in these directories are auto-discovered and their files can be read
// without permission prompts.
func GlobalSkillsDirs() []string {
	if skillsDir, ok := lookupCompatNonEmptyEnv("SKILLS_DIR"); ok {
		return []string{skillsDir}
	}

	var configBase string
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		configBase = xdgConfigHome
	} else if runtime.GOOS == "windows" {
		configBase = cmp.Or(
			os.Getenv("LOCALAPPDATA"),
			filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local"),
		)
	} else {
		configBase = filepath.Join(home.Dir(), ".config")
	}

	paths := []string{
		filepath.Join(configBase, RuntimeAppName(), "skills"),
		filepath.Join(configBase, alternateAppDirectory(RuntimeAppName()), "skills"),
		filepath.Join(configBase, "agents", "skills"),
	}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if !slices.Contains(result, path) {
			result = append(result, path)
		}
	}
	return result
}

func isAppleTerminal() bool { return os.Getenv("TERM_PROGRAM") == "Apple_Terminal" }

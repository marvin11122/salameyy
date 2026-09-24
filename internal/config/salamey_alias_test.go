package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeAppDefaultsToSalamey(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, "")
	originalArgs := os.Args
	os.Args = []string{"salamey"}
	t.Cleanup(func() { os.Args = originalArgs })

	require.Equal(t, appName, RuntimeAppName())
}

func TestConfigSetDefaultsUsesCrushDataDirectoryAlias(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	repoDir := t.TempDir()
	workDir := filepath.Join(repoDir, "nested", "project")

	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, defaultDataDirectoryAlias), 0o755))
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	cfg := &Config{}
	cfg.setDefaults(workDir, "")

	require.Equal(t, filepath.Join(repoDir, defaultDataDirectoryAlias), cfg.Options.DataDirectory)
	require.Contains(t, cfg.Options.ContextPaths, "SALAMEY.md")
	require.Contains(t, cfg.Options.ContextPaths, "SALAMEY.local.md")
}

func TestConfigSetDefaultsPrefersNearestDataDirectoryAlias(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	rootDir := t.TempDir()
	parentDir := filepath.Join(rootDir, "parent")
	repoDir := filepath.Join(parentDir, "repo")
	workDir := filepath.Join(repoDir, "nested", "project")

	require.NoError(t, os.MkdirAll(filepath.Join(parentDir, defaultDataDirectory), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, defaultDataDirectoryAlias), 0o755))
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	cfg := &Config{}
	cfg.setDefaults(workDir, "")

	require.Equal(t, filepath.Join(repoDir, defaultDataDirectoryAlias), cfg.Options.DataDirectory)
}

func TestConfigSetDefaultsPrefersRuntimeDataDirectoryWhenAliasesShareAncestor(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	repoDir := t.TempDir()
	workDir := filepath.Join(repoDir, "nested", "project")

	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, defaultDataDirectory), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, defaultDataDirectoryAlias), 0o755))
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	cfg := &Config{}
	cfg.setDefaults(workDir, "")

	require.Equal(t, filepath.Join(repoDir, defaultDataDirectoryAlias), cfg.Options.DataDirectory)
}

func TestConfigSetDefaultsPrefersSalameyDataDirectoryInSalameyModeWhenAliasesShareAncestor(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appName)
	repoDir := t.TempDir()
	workDir := filepath.Join(repoDir, "nested", "project")

	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, defaultDataDirectory), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, defaultDataDirectoryAlias), 0o755))
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	cfg := &Config{}
	cfg.setDefaults(workDir, "")

	require.Equal(t, filepath.Join(repoDir, defaultDataDirectory), cfg.Options.DataDirectory)
}

func TestLookupConfigsLoadsSalameyAliases(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))

	repoDir := t.TempDir()
	workDir := filepath.Join(repoDir, "nested", "project")
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(repoDir, configFileName(appAliasName)),
		[]byte(`{"options":{"initialize_as":"SALAMEY.md"}}`),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, "."+configFileName(appAliasName)),
		[]byte(`{"options":{"debug":true}}`),
		0o644,
	))

	cfg, err := loadFromConfigPaths(lookupConfigs(workDir))
	require.NoError(t, err)
	require.Equal(t, "SALAMEY.md", cfg.Options.InitializeAs)
	require.True(t, cfg.Options.Debug)
}

func TestLookupConfigsPrefersRuntimeConfigOverCompatFallback(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(repoDir, configFileName(appAliasName)),
		[]byte(`{"options":{"initialize_as":"SALAMEY.md"}}`),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(repoDir, configFileName(appName)),
		[]byte(`{"options":{"initialize_as":"CRUSH.md"}}`),
		0o644,
	))

	cfg, err := loadFromConfigPaths(lookupConfigs(repoDir))
	require.NoError(t, err)
	require.Equal(t, "SALAMEY.md", cfg.Options.InitializeAs)
}

func TestLookupConfigsPrefersCrushConfigInCrushMode(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appName)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(repoDir, configFileName(appAliasName)),
		[]byte(`{"options":{"initialize_as":"SALAMEY.md"}}`),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(repoDir, configFileName(appName)),
		[]byte(`{"options":{"initialize_as":"CRUSH.md"}}`),
		0o644,
	))

	cfg, err := loadFromConfigPaths(lookupConfigs(repoDir))
	require.NoError(t, err)
	require.Equal(t, "CRUSH.md", cfg.Options.InitializeAs)
}

func TestGlobalPathsHonorSalameyEnvAliases(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	configDir := filepath.Join(t.TempDir(), "config")
	dataDir := filepath.Join(t.TempDir(), "data")
	skillsDir := filepath.Join(t.TempDir(), "skills")

	t.Setenv("CRUSH_GLOBAL_CONFIG", "")
	t.Setenv("CRUSH_GLOBAL_DATA", "")
	t.Setenv("CRUSH_SKILLS_DIR", "")
	t.Setenv("SALAMEY_GLOBAL_CONFIG", configDir)
	t.Setenv("SALAMEY_GLOBAL_DATA", dataDir)
	t.Setenv("SALAMEY_SKILLS_DIR", skillsDir)

	require.Equal(t, filepath.Join(configDir, configFileName(appAliasName)), GlobalConfig())
	require.Equal(t, filepath.Join(dataDir, configFileName(appAliasName)), GlobalConfigData())
	require.Equal(t, []string{skillsDir}, GlobalSkillsDirs())
}

func TestGlobalPathsPreferCrushEnvAliasesInCrushMode(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	crushConfigDir := filepath.Join(t.TempDir(), "crush-config")
	salameyConfigDir := filepath.Join(t.TempDir(), "salamey-config")
	crushDataDir := filepath.Join(t.TempDir(), "crush-data")
	salameyDataDir := filepath.Join(t.TempDir(), "salamey-data")
	crushSkillsDir := filepath.Join(t.TempDir(), "crush-skills")
	salameySkillsDir := filepath.Join(t.TempDir(), "salamey-skills")

	t.Setenv("CRUSH_GLOBAL_CONFIG", crushConfigDir)
	t.Setenv("SALAMEY_GLOBAL_CONFIG", salameyConfigDir)
	t.Setenv("CRUSH_GLOBAL_DATA", crushDataDir)
	t.Setenv("SALAMEY_GLOBAL_DATA", salameyDataDir)
	t.Setenv("CRUSH_SKILLS_DIR", crushSkillsDir)
	t.Setenv("SALAMEY_SKILLS_DIR", salameySkillsDir)

	require.Equal(t, filepath.Join(crushConfigDir, configFileName(appAliasName)), GlobalConfig())
	require.Equal(t, filepath.Join(crushDataDir, configFileName(appAliasName)), GlobalConfigData())
	require.Equal(t, []string{crushSkillsDir}, GlobalSkillsDirs())
}

func TestGlobalPathsPreferSalameyEnvAliasesInSalameyMode(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appName)
	crushConfigDir := filepath.Join(t.TempDir(), "crush-config")
	salameyConfigDir := filepath.Join(t.TempDir(), "salamey-config")
	crushDataDir := filepath.Join(t.TempDir(), "crush-data")
	salameyDataDir := filepath.Join(t.TempDir(), "salamey-data")
	crushSkillsDir := filepath.Join(t.TempDir(), "crush-skills")
	salameySkillsDir := filepath.Join(t.TempDir(), "salamey-skills")

	t.Setenv("CRUSH_GLOBAL_CONFIG", crushConfigDir)
	t.Setenv("SALAMEY_GLOBAL_CONFIG", salameyConfigDir)
	t.Setenv("CRUSH_GLOBAL_DATA", crushDataDir)
	t.Setenv("SALAMEY_GLOBAL_DATA", salameyDataDir)
	t.Setenv("CRUSH_SKILLS_DIR", crushSkillsDir)
	t.Setenv("SALAMEY_SKILLS_DIR", salameySkillsDir)

	require.Equal(t, filepath.Join(salameyConfigDir, configFileName(appName)), GlobalConfig())
	require.Equal(t, filepath.Join(salameyDataDir, configFileName(appName)), GlobalConfigData())
	require.Equal(t, []string{salameySkillsDir}, GlobalSkillsDirs())
}

func TestGlobalPathsDefaultToRuntimeIdentity(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))

	require.Contains(t, GlobalConfig(), filepath.Join("crush", "crush.json"))
	require.Contains(t, GlobalConfigData(), filepath.Join("crush", "crush.json"))
	require.Contains(t, GlobalSkillsDirs()[0], filepath.Join("crush", "skills"))
}

func TestGlobalConfigPathsIncludesLegacyDirectories(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appName)
	t.Setenv("SALAMEY_GLOBAL_CONFIG", "")
	t.Setenv("CRUSH_GLOBAL_CONFIG", "")
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	paths := globalConfigPaths(GlobalConfig())
	require.Contains(t, paths, filepath.Join(configHome, "salamey", "crush.json"))
	require.Contains(t, paths, filepath.Join(configHome, "crush", "crush.json"))
	require.Equal(t, filepath.Join(configHome, "salamey", "salamey.json"), paths[len(paths)-1])
}

func TestConfigSetDefaultsHonorsSalameyDisableEnvAliases(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	t.Setenv("SALAMEY_DISABLE_PROVIDER_AUTO_UPDATE", "true")
	t.Setenv("SALAMEY_DISABLE_DEFAULT_PROVIDERS", "true")

	cfg := &Config{}
	cfg.setDefaults(t.TempDir(), "")

	require.True(t, cfg.Options.DisableProviderAutoUpdate)
	require.True(t, cfg.Options.DisableDefaultProviders)
}

func TestPushPopAppEnvMirrorsSalameyAliases(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	t.Setenv("OPENAI_API_KEY", "original")
	t.Setenv("SALAMEY_OPENAI_API_KEY", "salamey-key")

	restore := PushPopAppEnv()
	require.Equal(t, "salamey-key", os.Getenv("OPENAI_API_KEY"))

	restore()
	require.Equal(t, "original", os.Getenv("OPENAI_API_KEY"))
}

func TestPushPopAppEnvPrefersCrushAliasesInCrushMode(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appAliasName)
	t.Setenv("CRUSH_OPENAI_API_KEY", "crush-key")
	t.Setenv("SALAMEY_OPENAI_API_KEY", "salamey-key")

	restore := PushPopAppEnv()
	require.Equal(t, "crush-key", os.Getenv("OPENAI_API_KEY"))
	restore()
}

func TestPushPopAppEnvPrefersSalameyAliasesInSalameyMode(t *testing.T) {
	t.Setenv(runtimeIdentityEnv, appName)
	t.Setenv("CRUSH_OPENAI_API_KEY", "crush-key")
	t.Setenv("SALAMEY_OPENAI_API_KEY", "salamey-key")

	restore := PushPopAppEnv()
	require.Equal(t, "salamey-key", os.Getenv("OPENAI_API_KEY"))
	restore()
}

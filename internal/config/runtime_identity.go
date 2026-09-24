package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const runtimeIdentityEnv = "SALAMEY_APP_IDENTITY"

// RuntimeAppName returns the active application identity.
// SALAMEY is the default unless the executable name or explicit environment
// override selects crush compatibility mode.
func RuntimeAppName() string {
	if identity := normalizedRuntimeIdentity(os.Getenv(runtimeIdentityEnv)); identity != "" {
		return identity
	}

	base := strings.ToLower(filepath.Base(os.Args[0]))
	switch {
	case strings.Contains(base, appAliasName):
		return appAliasName
	case strings.Contains(base, appName):
		return appName
	default:
		return appName
	}
}

// RuntimeFallbackAppName returns the compatibility identity used as fallback.
func RuntimeFallbackAppName() string {
	if RuntimeAppName() == appAliasName {
		return appName
	}
	return appAliasName
}

// RuntimeDataDirectoryName returns the default project-local data directory.
func RuntimeDataDirectoryName() string {
	if RuntimeAppName() == appAliasName {
		return defaultDataDirectoryAlias
	}
	return defaultDataDirectory
}

// RuntimeFallbackDataDirectoryName returns the compatibility data directory.
func RuntimeFallbackDataDirectoryName() string {
	if RuntimeDataDirectoryName() == defaultDataDirectoryAlias {
		return defaultDataDirectory
	}
	return defaultDataDirectoryAlias
}

// RuntimeLogFileName returns the active log filename.
func RuntimeLogFileName() string {
	return RuntimeAppName() + ".log"
}

// RuntimeDatabaseFileName returns the active database filename.
func RuntimeDatabaseFileName() string {
	return RuntimeAppName() + ".db"
}

func RuntimeFallbackDatabaseFileName() string {
	return RuntimeFallbackAppName() + ".db"
}

func LookupCompatBoolEnv(suffix string) (bool, bool) {
	value, ok := lookupCompatEnv(suffix)
	if !ok {
		return false, false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, false
	}
	return parsed, true
}

func lookupCompatEnv(suffix string) (string, bool) {
	primary, fallback := compatEnvNames(suffix)
	return lookupEnv(primary, fallback)
}

func lookupCompatNonEmptyEnv(suffix string) (string, bool) {
	primary, fallback := compatEnvNames(suffix)
	return lookupNonEmptyEnv(primary, fallback)
}

func preferredCompatConfigFileName(suffix string) string {
	primary, fallback := compatEnvNames(suffix)
	if _, ok := lookupNonEmptyEnv(primary, ""); ok {
		return configFileName(RuntimeAppName())
	}
	if _, ok := lookupNonEmptyEnv(fallback, ""); ok {
		return configFileName(RuntimeFallbackAppName())
	}
	return configFileName(RuntimeAppName())
}

func compatEnvNames(suffix string) (primary string, fallback string) {
	primary = strings.ToUpper(RuntimeAppName()) + "_" + suffix
	fallback = strings.ToUpper(RuntimeFallbackAppName()) + "_" + suffix
	return primary, fallback
}

func normalizedRuntimeIdentity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case appName:
		return appName
	case appAliasName:
		return appAliasName
	default:
		return ""
	}
}

package version

import "runtime/debug"

// Build-time parameters set via -ldflags

var Version = "devel"

// A user may install SALAMEY using `go install github.com/wiesnermarven-hue/salameyy@latest`.
// The module basename is salameyy; use `go build -o salamey .` or a release
// artifact when the canonical executable name is required. Without -ldflags,
// Version remains unset, so the embedded build version is used as a fallback.
func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	mainVersion := info.Main.Version
	if mainVersion != "" && mainVersion != "(devel)" {
		Version = mainVersion
	}
}

// Package version provides version information for Ralph.
//
// Version information is set at build time using ldflags:
//
//	go build -ldflags "-X github.com/patbaumgartner/copilot-ralph/pkg/version.Version=1.0.0"
//
// When built via "go install" (no ldflags), the module version embedded by the
// Go toolchain is read from runtime/debug.ReadBuildInfo as a fallback.
package version

import (
	"runtime/debug"
	"strings"
)

// Build-time variables set via ldflags
var (
	// Version is the semantic version number
	Version = "dev"

	// Commit is the git commit hash
	Commit = "unknown"

	// BuildDate is the build timestamp
	BuildDate = "unknown"

	// GoVersion is the Go version used to build
	GoVersion = "unknown"
)

func init() {
	if Version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		Version = strings.TrimPrefix(v, "v")
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) > 7 {
				Commit = s.Value[:7]
			} else {
				Commit = s.Value
			}
		case "vcs.time":
			BuildDate = s.Value
		}
	}
	if info.GoVersion != "" {
		GoVersion = info.GoVersion
	}
}

// Info contains version information
type Info struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
}

// Get returns the version information
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: GoVersion,
	}
}

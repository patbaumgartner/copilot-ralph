package version

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetReturnsSetValues(t *testing.T) {
	oldV, oldC, oldD, oldG := Version, Commit, BuildDate, GoVersion
	defer func() {
		Version = oldV
		Commit = oldC
		BuildDate = oldD
		GoVersion = oldG
	}()

	Version = "1.2.3"
	Commit = "abc123"
	BuildDate = "2026-01-01"
	GoVersion = "go1.20"

	info := Get()
	assert.Equal(t, "1.2.3", info.Version)
	assert.Equal(t, "abc123", info.Commit)
	assert.Equal(t, "2026-01-01", info.BuildDate)
	assert.Equal(t, "go1.20", info.GoVersion)
}

func TestApplyBuildInfo(t *testing.T) {
	oldV, oldC, oldD, oldG := Version, Commit, BuildDate, GoVersion
	defer func() {
		Version = oldV
		Commit = oldC
		BuildDate = oldD
		GoVersion = oldG
	}()

	Version = "dev"
	Commit = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"

	applyBuildInfo(&debug.BuildInfo{
		GoVersion: "go1.26.2",
		Main:      debug.Module{Version: "v1.2.3"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.time", Value: "2026-04-27T20:00:00Z"},
			{Key: "other", Value: "ignored"},
		},
	})

	assert.Equal(t, "1.2.3", Version)
	assert.Equal(t, "abcdef1", Commit)
	assert.Equal(t, "2026-04-27T20:00:00Z", BuildDate)
	assert.Equal(t, "go1.26.2", GoVersion)
}

func TestApplyBuildInfoLeavesDefaultsForDevelopmentBuild(t *testing.T) {
	oldV, oldC, oldD, oldG := Version, Commit, BuildDate, GoVersion
	defer func() {
		Version = oldV
		Commit = oldC
		BuildDate = oldD
		GoVersion = oldG
	}()

	Version = "dev"
	Commit = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"

	applyBuildInfo(nil)
	applyBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc"},
		},
	})

	assert.Equal(t, "dev", Version)
	assert.Equal(t, "abc", Commit)
	assert.Equal(t, "unknown", BuildDate)
	assert.Equal(t, "unknown", GoVersion)
}

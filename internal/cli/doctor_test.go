// Package cli — tests for `ralph doctor`.
package cli

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckRalphVersion(t *testing.T) {
	c := checkRalphVersion()
	assert.Equal(t, "ralph", c.Name)
	assert.True(t, c.Ok)
	assert.NotEmpty(t, c.Status)
}

func TestCheckBinaryFound(t *testing.T) {
	// "git" is available in the CI environment.
	c := checkBinary("git")
	assert.Equal(t, "git", c.Name)
	assert.True(t, c.Ok)
	assert.NotEqual(t, "missing", c.Status)
}

func TestCheckBinaryMissing(t *testing.T) {
	c := checkBinary("__totally_nonexistent_ralph_test_binary__")
	assert.Equal(t, "__totally_nonexistent_ralph_test_binary__", c.Name)
	assert.False(t, c.Ok)
	assert.Equal(t, "missing", c.Status)
	assert.NotEmpty(t, c.Detail)
}

func TestCheckWorkingDirWritable(t *testing.T) {
	// Change to a known-writable temp dir so the check succeeds
	// regardless of the CI cwd permissions.
	dir := t.TempDir()
	old, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(old) })

	// os.Getwd() resolves symlinks (e.g. /var → /private/var on macOS),
	// so use it to get the canonical expected path instead of t.TempDir().
	want, err := os.Getwd()
	require.NoError(t, err)

	c := checkWorkingDir()
	assert.Equal(t, "working-dir", c.Name)
	assert.True(t, c.Ok)
	assert.Equal(t, want, c.Status)
}

func TestRunDoctorNoFailures(t *testing.T) {
	// runDoctor is expected to succeed when git is present and the cwd is
	// writable. copilot may be absent; that makes the doctor fail, so we
	// only assert the function returns without panicking and that any error
	// is an *ExitError.
	cmd := &cobra.Command{}
	err := runDoctor(cmd, nil)
	if err != nil {
		var exitErr *ExitError
		require.ErrorAs(t, err, &exitErr)
		assert.Equal(t, exitFailed, exitErr.Code)
	}
}

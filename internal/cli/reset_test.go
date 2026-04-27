// Package cli — tests for `ralph reset`.
package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunResetMissingFlag(t *testing.T) {
	old := resetCheckpoint
	resetCheckpoint = ""
	t.Cleanup(func() { resetCheckpoint = old })

	err := runReset(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--checkpoint-file is required")
}

func TestRunResetDeletesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1}`), 0o600))

	old := resetCheckpoint
	resetCheckpoint = path
	t.Cleanup(func() { resetCheckpoint = old })

	err := runReset(nil, nil)
	require.NoError(t, err)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "checkpoint file should have been deleted")
}

func TestRunResetForceIgnoresMissingFile(t *testing.T) {
	old := resetCheckpoint
	oldForce := resetForce
	resetCheckpoint = filepath.Join(t.TempDir(), "does_not_exist.json")
	resetForce = true
	t.Cleanup(func() {
		resetCheckpoint = old
		resetForce = oldForce
	})

	// checkpoint.Delete treats missing files as success, so force or not
	// the call must succeed.
	err := runReset(nil, nil)
	require.NoError(t, err)
}

func TestRunResetNonForceOnMissingFileSucceeds(t *testing.T) {
	// checkpoint.Delete returns nil for missing files (idempotent),
	// so non-force mode also succeeds.
	old := resetCheckpoint
	oldForce := resetForce
	resetCheckpoint = filepath.Join(t.TempDir(), "does_not_exist.json")
	resetForce = false
	t.Cleanup(func() {
		resetCheckpoint = old
		resetForce = oldForce
	})

	err := runReset(nil, nil)
	require.NoError(t, err)
}

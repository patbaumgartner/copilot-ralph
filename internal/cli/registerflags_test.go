package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePromptStack(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "p.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("file body"), 0o600))

	tests := []struct {
		name    string
		entries []string
		want    []string
		wantErr bool
	}{
		{name: "empty", entries: nil, want: []string{}},
		{name: "skips blank", entries: []string{"", "  "}, want: []string{}},
		{name: "literal", entries: []string{"hello"}, want: []string{"hello"}},
		{name: "file", entries: []string{mdPath}, want: []string{"file body"}},
		{name: "mixed", entries: []string{"  ", "x", mdPath}, want: []string{"x", "file body"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePromptStack(tt.entries)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRegisterRunFlagsBindsKeyFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "fake"}
	registerRunFlags(cmd)

	for _, name := range []string{
		"max-iterations", "timeout", "promise", "model", "working-dir",
		"dry-run", "streaming", "system-prompt", "system-prompt-mode",
		"checkpoint-file", "oracle-model", "prompt-stack", "no-rate-limit-wait",
		"json", "json-output", "log-file", "webhook",
		// New flags
		"blocked-phrase", "stall-after", "iteration-delay",
		"on-complete", "on-blocked",
	} {
		assert.NotNilf(t, cmd.Flags().Lookup(name), "expected flag --%s", name)
	}
}

func TestResumeAndRunShareFlags(t *testing.T) {
	// Both run and resume must accept every loop flag.
	for _, name := range []string{"max-iterations", "checkpoint-file", "oracle-model"} {
		assert.NotNilf(t, runCmd.Flags().Lookup(name), "run missing --%s", name)
		assert.NotNilf(t, resumeCmd.Flags().Lookup(name), "resume missing --%s", name)
	}
}

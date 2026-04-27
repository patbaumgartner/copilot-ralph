package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletionCommandRegistered(t *testing.T) {
	require.NotNil(t, completionCmd, "completionCmd must be defined")
	assert.Equal(t, "completion [bash|zsh|fish|powershell]", completionCmd.Use)
}

func TestCompletionCommandInRootCmd(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == completionCmd.Use {
			found = true
			break
		}
	}
	assert.True(t, found, "completion command must be registered on rootCmd")
}

func TestCompletionCommandValidArgs(t *testing.T) {
	// Verify all four shells are listed as valid args.
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		found := false
		for _, a := range completionCmd.ValidArgs {
			if a == shell {
				found = true
				break
			}
		}
		assert.Truef(t, found, "expected %q in ValidArgs", shell)
	}
}

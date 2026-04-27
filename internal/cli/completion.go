// Package cli implements the command-line interface for Ralph using Cobra.
//
// This file adds the `ralph completion <shell>` command that emits shell
// completion scripts for bash, zsh, fish, and powershell.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// completionCmd generates shell completion scripts for the supported shells.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for the specified shell.

To load completions:

Bash:
  source <(ralph completion bash)

  # To load completions for each session, execute once:
  # Linux:
  ralph completion bash > /etc/bash_completion.d/ralph
  # macOS:
  ralph completion bash > $(brew --prefix)/etc/bash_completion.d/ralph

Zsh:
  # If shell completion is not already enabled in your environment,
  # enable it by executing the following once:
  echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  ralph completion zsh > "${fpath[1]}/_ralph"

  # Start a new shell for this setup to take effect.

Fish:
  ralph completion fish | source

  # To load completions for each session, execute once:
  ralph completion fish > ~/.config/fish/completions/ralph.fish

PowerShell:
  ralph completion powershell | Out-String | Invoke-Expression

  # To load completions for each session, add the above line to your profile.`,
	ValidArgs:     []string{"bash", "zsh", "fish", "powershell"},
	Args:          cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell: %q", args[0])
		}
	},
}

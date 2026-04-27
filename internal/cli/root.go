// Package cli implements the command-line interface for Ralph using Cobra.
//
// This package defines every Ralph subcommand (run, resume, reset, doctor,
// version) and wires their flags into a core.LoopConfig before delegating
// to the loop engine.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/patbaumgartner/copilot-ralph/pkg/version"
)

var (
	// noColor disables colored output
	noColor bool

	// rootCmd is the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:   "ralph",
		Short: "Ralph - Iterative AI Development Loop Tool",
		Long: `Ralph implements the "Ralph Wiggum" technique: drive an iterative
loop against the GitHub Copilot SDK until the assistant signals completion,
hits --max-iterations, hits --timeout, or you press Ctrl+C.`,
		Version: version.Version,
	}
)

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")

	// Add subcommands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(completionCmd)
}

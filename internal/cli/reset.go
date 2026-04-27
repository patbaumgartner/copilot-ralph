// Package cli — `ralph reset` clears Ralph state files (currently the
// checkpoint). It is a small convenience around checkpoint.Delete.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/patbaumgartner/copilot-ralph/internal/checkpoint"
)

var (
	resetCheckpoint string
	resetForce      bool
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset Ralph state (checkpoint files)",
	Long: `Reset Ralph state by deleting the named checkpoint file.

Currently --checkpoint-file is the only state Ralph keeps; reset removes
it so the next run starts from scratch.`,
	RunE:          runReset,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	resetCmd.Flags().StringVar(&resetCheckpoint, "checkpoint-file", "", "checkpoint file to delete (required)")
	resetCmd.Flags().BoolVar(&resetForce, "force", false, "do not error when the checkpoint file does not exist")
}

func runReset(cmd *cobra.Command, args []string) error {
	_ = cmd
	_ = args
	if resetCheckpoint == "" {
		return fmt.Errorf("--checkpoint-file is required")
	}
	if err := checkpoint.Delete(resetCheckpoint); err != nil {
		if resetForce {
			return nil
		}
		return fmt.Errorf("delete checkpoint: %w", err)
	}
	fmt.Printf("removed %s\n", resetCheckpoint)
	return nil
}

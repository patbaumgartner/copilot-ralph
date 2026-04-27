// Package cli — `ralph resume <checkpoint>` continues a previously
// checkpointed loop. It reads the checkpoint, copies most of its values
// into LoopConfig (so flags can override), and starts the engine with
// the saved iteration counter and carry-context summary.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/patbaumgartner/copilot-ralph/internal/checkpoint"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <checkpoint-file>",
	Short: "Resume a previously checkpointed Ralph loop",
	Long: `Read the named checkpoint file and continue the loop where it left off.

The checkpoint provides the prompt, working directory, model, and the last
carry-context summary. Most ralph run flags are honored; --max-iterations
and --timeout default to their normal values unless overridden.`,
	Args:          cobra.ExactArgs(1),
	RunE:          runResume,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// Resume registers its own flag set (independent of init order)
	// so every `ralph run` flag is also accepted on `ralph resume`.
	registerRunFlags(resumeCmd)
}

// runResume loads the checkpoint and delegates to the same path as
// `ralph run`, with the loop configuration primed from the saved state.
func runResume(cmd *cobra.Command, args []string) error {
	state, err := checkpoint.Load(args[0])
	if err != nil {
		return fmt.Errorf("load checkpoint: %w", err)
	}

	if state.Prompt == "" {
		return errors.New("checkpoint has no prompt")
	}

	cfg, err := buildLoopConfig(state.Prompt)
	if err != nil {
		return fmt.Errorf("build loop config: %w", err)
	}

	// Restore values that the user did not explicitly override on the
	// command line. We treat empty/zero fields as "use the checkpoint".
	if !cmd.Flags().Changed("model") && state.Model != "" {
		cfg.Model = state.Model
	}
	if !cmd.Flags().Changed("working-dir") && state.WorkingDir != "" {
		cfg.WorkingDir = state.WorkingDir
	}
	if !cmd.Flags().Changed("promise") && state.PromisePhrase != "" {
		cfg.PromisePhrase = state.PromisePhrase
	}
	if !cmd.Flags().Changed("max-iterations") && state.MaxIterations > 0 {
		cfg.MaxIterations = state.MaxIterations
	}
	cfg.ResumeFromIteration = state.Iteration
	cfg.ResumeSummary = state.LastSummary

	// Default the checkpoint flag to the file we just loaded so the
	// resumed run keeps writing back to the same place.
	if cfg.CheckpointFile == "" {
		cfg.CheckpointFile = args[0]
	}

	return runLoopWithConfig(cmd, cfg)
}

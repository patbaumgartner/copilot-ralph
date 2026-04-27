// Package cli implements the command-line interface for Ralph using Cobra.
//
// This file implements the `ralph run` command, including flag wiring, event
// display, and loop lifecycle management. Checkpoint/resume logic lives in
// resume.go; environment checks in doctor.go.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/patbaumgartner/copilot-ralph/internal/core"
	"github.com/patbaumgartner/copilot-ralph/internal/eventsink"
	"github.com/patbaumgartner/copilot-ralph/internal/oracle"
	"github.com/patbaumgartner/copilot-ralph/internal/sdk"
	"github.com/patbaumgartner/copilot-ralph/internal/tui/styles"
)

// Exit codes per spec
const (
	exitSuccess       = 0
	exitFailed        = 1
	exitCancelled     = 2
	exitTimeout       = 3
	exitMaxIterations = 4
	exitBlocked       = 5
)

// ExitError carries an exit code that main can translate to the process exit
// status while still letting Cobra/normal control flow run deferred cleanup.
type ExitError struct {
	Err  error
	Code int
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run <prompt>",
	Short: "Run an AI development loop",
	Long: `Run an AI development loop with a prompt.

The loop will iterate until the AI outputs the completion promise phrase,
reaches the maximum iterations, or times out.

Examples:
  # Direct prompt (required)
  ralph run "Add unit tests for the parser module"

  # Using a Markdown file as prompt
  ralph run task_description.md

  # With options
  ralph run --max-iterations 5 --timeout 10m "Refactor authentication"

  # Dry run
  ralph run --dry-run "Update documentation"

  # Override promise phrase (must be wrapped in <promise>...</promise> by the assistant)
  ralph run --promise "Task complete!" "Fix bug"`,
	Args:          cobra.ExactArgs(1),
	RunE:          runLoop,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	runMaxIterations    int
	runTimeout          time.Duration
	runIterTimeout      time.Duration
	runPromise          string
	runModel            string
	runWorkingDir       string
	runDryRun           bool
	runStreaming        bool
	runSystemPrompt     string
	runSystemPromptMode string
	runLogLevel         string
	runCarryContext     string
	runCarryMaxRunes    int
	runPlanFile         string
	runSpecsDir         string
	runStopOnNoChanges  int
	runStopOnError      int
	runVerifyCmd        string
	runVerifyTimeout    time.Duration
	runVerifyMaxBytes   int
	runAutoCommit       bool
	runAutoCommitMsg    string
	runAutoCommitForce  bool
	runAutoTag          string
	runDiffStat         bool

	// Structured outputs.
	runJSON           bool
	runJSONOutput     string
	runLogFile        string
	runWebhook        string
	runWebhookTimeout time.Duration

	// Checkpoint / resume.
	runCheckpoint string

	// Oracle second-opinion.
	runOracleModel       string
	runOracleEvery       int
	runOracleOnVerifyErr bool

	// Prompt stack and rate-limit handling.
	runPromptStack     []string
	runNoRateLimitWait bool

	// Blocked signal, stall detection, iteration delay.
	runBlockedPhrase  string
	runStallAfter     int
	runIterationDelay time.Duration

	// Lifecycle hooks.
	runOnComplete string
	runOnBlocked  string
)

func init() {
	registerRunFlags(runCmd)
}

// registerRunFlags wires every loop-related flag onto the supplied command.
// It is shared by `ralph run` and `ralph resume` so both commands accept
// the same overrides regardless of file init order.
func registerRunFlags(cmd *cobra.Command) {
	cmd.Flags().IntVarP(&runMaxIterations, "max-iterations", "m", envInt("RALPH_MAX_ITERATIONS", 10), "maximum loop iterations")
	cmd.Flags().DurationVarP(&runTimeout, "timeout", "t", envDuration("RALPH_TIMEOUT", 30*time.Minute), "maximum loop runtime")
	cmd.Flags().DurationVar(&runIterTimeout, "iteration-timeout", envDuration("RALPH_ITERATION_TIMEOUT", 0), "per-iteration soft timeout (0 disables)")
	cmd.Flags().StringVar(&runPromise, "promise", envString("RALPH_PROMISE", "I'm special!"), "completion promise phrase")
	cmd.Flags().StringVar(&runModel, "model", envString("RALPH_MODEL", "gpt-4"), "AI model to use")
	cmd.Flags().StringVar(&runWorkingDir, "working-dir", envString("RALPH_WORKING_DIR", "."), "working directory for loop execution")
	cmd.Flags().BoolVar(&runDryRun, "dry-run", false, "show what would be executed without running")
	cmd.Flags().BoolVar(&runStreaming, "streaming", envBool("RALPH_STREAMING", true), "enable streaming responses")
	cmd.Flags().StringVar(&runSystemPrompt, "system-prompt", envString("RALPH_SYSTEM_PROMPT", ""), "custom system message, can be a prompt or path to Markdown file")
	cmd.Flags().StringVar(&runSystemPromptMode, "system-prompt-mode", "append", "system message mode: append or replace")
	cmd.Flags().StringVar(&runLogLevel, "log-level", "info", "log level: debug, info, warn, error")
	cmd.Flags().StringVar(&runCarryContext, "carry-context", envString("RALPH_CARRY_CONTEXT", "summary"), "carry-context mode: off, summary, verbatim")
	cmd.Flags().IntVar(&runCarryMaxRunes, "carry-context-max-runes", 4000, "maximum runes carried into the next iteration prompt (<=0 disables truncation)")
	cmd.Flags().StringVar(&runPlanFile, "plan-file", "", "path to running fix_plan.md (relative paths resolved against --working-dir; empty disables)")
	cmd.Flags().StringVar(&runSpecsDir, "specs", "", "directory whose Markdown specs are listed in each iteration prompt")
	cmd.Flags().IntVar(&runStopOnNoChanges, "stop-on-no-changes", 0, "halt after N consecutive iterations with no git working-tree changes (0 disables)")
	cmd.Flags().IntVar(&runStopOnError, "stop-on-error", 0, "halt after N consecutive iterations that emit at least one error event (0 disables)")
	cmd.Flags().StringVar(&runVerifyCmd, "verify-cmd", envString("RALPH_VERIFY_CMD", ""), "shell command run after each iteration; failures fold into the next prompt")
	cmd.Flags().DurationVar(&runVerifyTimeout, "verify-timeout", 5*time.Minute, "timeout for a single --verify-cmd run")
	cmd.Flags().IntVar(&runVerifyMaxBytes, "verify-max-bytes", 16*1024, "max bytes captured per stream from --verify-cmd (<=0 unlimited)")
	cmd.Flags().BoolVar(&runAutoCommit, "auto-commit", false, "git add -A && git commit after each iteration that produced changes (never pushes)")
	cmd.Flags().StringVar(&runAutoCommitMsg, "auto-commit-message", "ralph: iteration %d", "format string for auto-commit messages (%d for iteration number)")
	cmd.Flags().BoolVar(&runAutoCommitForce, "auto-commit-on-failure", false, "auto-commit even when --verify-cmd failed (default: only on success)")
	cmd.Flags().StringVar(&runAutoTag, "auto-tag", "", "format string for an annotated tag created on each auto-commit (e.g. \"ralph/iter-%d\")")
	cmd.Flags().BoolVar(&runDiffStat, "diff-stat", false, "emit `git diff --stat HEAD` as a WorkspaceDiffEvent each iteration")

	// JSON output, log file, webhook.
	cmd.Flags().BoolVar(&runJSON, "json", false, "emit JSON Lines (one envelope per event) to stdout instead of styled output")
	cmd.Flags().StringVar(&runJSONOutput, "json-output", "", "in addition to styled output, write JSON Lines to this file")
	cmd.Flags().StringVar(&runLogFile, "log-file", "", "append a one-line summary of every event to this file")
	cmd.Flags().StringVar(&runWebhook, "webhook", "", "POST every event as JSON to this URL")
	cmd.Flags().DurationVar(&runWebhookTimeout, "webhook-timeout", 5*time.Second, "timeout for a single --webhook delivery")

	// Checkpoint.
	cmd.Flags().StringVar(&runCheckpoint, "checkpoint-file", envString("RALPH_CHECKPOINT_FILE", ""), "write loop state to this file after every iteration (resume with `ralph resume`)")

	// Oracle.
	cmd.Flags().StringVar(&runOracleModel, "oracle-model", envString("RALPH_ORACLE_MODEL", ""), "second-opinion model consulted between iterations (empty disables)")
	cmd.Flags().IntVar(&runOracleEvery, "oracle-every", 0, "consult the oracle every N iterations (<=0 disables)")
	cmd.Flags().BoolVar(&runOracleOnVerifyErr, "oracle-on-verify-fail", false, "consult the oracle whenever --verify-cmd fails")

	// Prompt-stack and rate-limit handling.
	cmd.Flags().StringSliceVar(&runPromptStack, "prompt-stack", nil, "additional prompts (paths or literals) prepended to the main prompt in order")
	cmd.Flags().BoolVar(&runNoRateLimitWait, "no-rate-limit-wait", envBool("RALPH_NO_RATE_LIMIT_WAIT", false), "fail immediately on Copilot rate-limit / quota errors instead of waiting")

	// Blocked signal, stall detection, iteration delay.
	cmd.Flags().StringVar(&runBlockedPhrase, "blocked-phrase", envString("RALPH_BLOCKED_PHRASE", ""), "halt with exit 5 when assistant wraps this phrase in <blocked>...</blocked>")
	cmd.Flags().IntVar(&runStallAfter, "stall-after", envInt("RALPH_STALL_AFTER", 0), "halt after N consecutive iterations with identical responses (0 disables)")
	cmd.Flags().DurationVar(&runIterationDelay, "iteration-delay", envDuration("RALPH_ITERATION_DELAY", 0), "pause between iterations (0 disables)")

	// Lifecycle hooks.
	cmd.Flags().StringVar(&runOnComplete, "on-complete", envString("RALPH_ON_COMPLETE", ""), "shell command run after the loop completes successfully")
	cmd.Flags().StringVar(&runOnBlocked, "on-blocked", envString("RALPH_ON_BLOCKED", ""), "shell command run when the model emits the blocked signal")
}

// runLoop executes the AI development loop.
func runLoop(cmd *cobra.Command, args []string) error {
	// Resolve prompt from positional argument (text or path to .md/.markdown).
	prompt, err := resolvePrompt(args[0])
	if err != nil {
		return fmt.Errorf("resolve prompt: %w", err)
	}

	if prompt == "" {
		return errors.New("prompt is required")
	}

	// Build loop configuration from flags
	loopConfig, err := buildLoopConfig(prompt)
	if err != nil {
		return fmt.Errorf("build loop config: %w", err)
	}
	return runLoopWithConfig(cmd, loopConfig)
}

// runLoopWithConfig drives the engine with an already-built LoopConfig.
// It exists so `ralph resume` can inject a checkpoint-derived config
// without re-resolving a prompt argument.
func runLoopWithConfig(cmd *cobra.Command, loopConfig *core.LoopConfig) error {
	_ = cmd

	// Validate configuration
	if err := validateRunConfig(loopConfig); err != nil {
		return err
	}

	if err := validateSettings(loopConfig); err != nil {
		return err
	}

	// Warn when a custom system prompt in replace mode strips the
	// `<promise>...</promise>` instruction the engine relies on.
	if loopConfig.SystemPrompt != "" && loopConfig.SystemPromptMode == "replace" {
		fmt.Fprintln(os.Stderr, styles.WarningStyle.Render(
			"⚠ --system-prompt-mode=replace removes Ralph's promise instructions. "+
				"Promise detection will not fire unless your custom prompt instructs the assistant "+
				"to wrap the completion phrase in <promise>...</promise>."))
	}

	// Handle dry run
	if loopConfig.DryRun {
		return printDryRun(loopConfig)
	}

	// Print configuration
	printLoopConfig(loopConfig)

	// Create SDK client
	sdkClient, err := createSDKClient(loopConfig)
	if err != nil {
		return fmt.Errorf("failed to create SDK client: %w", err)
	}
	defer func() { _ = sdkClient.Stop() }()

	// Start SDK client
	if err := sdkClient.Start(); err != nil {
		return fmt.Errorf("failed to start SDK client: %w", err)
	}

	// Create loop engine
	engine := core.NewLoopEngine(loopConfig, sdkClient)

	// Optionally attach a second-opinion oracle.
	if loopConfig.OracleModel != "" {
		o, oerr := oracle.New(loopConfig.OracleModel, loopConfig.WorkingDir)
		if oerr != nil {
			fmt.Fprintln(os.Stderr, styles.WarningStyle.Render(fmt.Sprintf("⚠ oracle disabled: %v", oerr)))
		}
		if oerr == nil {
			engine.SetOracle(o)
			defer func() { _ = o.Close() }()
		}
	}

	// Build the event sink fan-out.
	sinks, sinkErr := buildEventSinks()
	if sinkErr != nil {
		return fmt.Errorf("event sinks: %w", sinkErr)
	}
	defer func() { _ = sinks.Close() }()

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// SIGQUIT (Ctrl+\) flushes the latest checkpoint and triggers a
	// graceful shutdown when checkpointing is enabled.
	quitCh := make(chan os.Signal, 1)
	if loopConfig.CheckpointFile != "" {
		signal.Notify(quitCh, syscall.SIGQUIT)
		defer signal.Stop(quitCh)
	}

	// Start event listener to display progress
	startTime := time.Now()
	eventsDone := make(chan struct{})
	go func() {
		displayEventsWithSinks(engine.Events(), loopConfig, sinks)
		close(eventsDone)
	}()

	// Start the loop in a goroutine
	resultCh := make(chan *core.LoopResult, 1)
	go func() {
		result, _ := engine.Start(ctx)
		resultCh <- result
	}()

	var result *core.LoopResult

	select {
	case <-sigCh:
		fmt.Println(styles.WarningStyle.Render("\n⚠ Received interrupt signal, cancelling loop..."))
		signal.Stop(sigCh)
		cancel()

		// Wait for the loop to finish. A second interrupt returns via RunE so
		// main() can translate the exit code without bypassing deferred cleanup.
		forceCh := make(chan os.Signal, 1)
		signal.Notify(forceCh, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(forceCh)
		select {
		case result = <-resultCh:
		case <-forceCh:
			fmt.Println(styles.ErrorStyle.Render("\n⚠ Second interrupt received, forcing exit..."))
			return &ExitError{Code: exitCancelled}
		}
	case <-quitCh:
		fmt.Println(styles.WarningStyle.Render(
			fmt.Sprintf("\n⚠ Received SIGQUIT, saving checkpoint to %s and stopping...", loopConfig.CheckpointFile)))
		signal.Stop(quitCh)
		cancel()
		result = <-resultCh
	case result = <-resultCh:
	}

	// Wait for events to finish displaying (with timeout to prevent hanging)
	select {
	case <-eventsDone:
	case <-time.After(1 * time.Second):
	}

	// Surface any sink delivery errors so the user is not left wondering
	// why their webhook never received an event.
	for _, sErr := range sinks.Errors() {
		fmt.Fprintln(os.Stderr, styles.WarningStyle.Render(fmt.Sprintf("⚠ event sink: %v", sErr)))
	}

	if result != nil {
		printSummary(result, startTime)
	}

	// Run lifecycle hooks before returning the exit error.
	if result != nil {
		switch result.State {
		case core.StateComplete:
			runHook(loopConfig.OnCompleteCmd, loopConfig.WorkingDir, result)
		case core.StateBlocked:
			runHook(loopConfig.OnBlockedCmd, loopConfig.WorkingDir, result)
		}
	}

	return exitErrorFor(result)
}

// exitErrorFor maps a loop result to an ExitError so main() can translate it
// to the process exit code without bypassing deferred cleanup.
func exitErrorFor(result *core.LoopResult) error {
	if result == nil {
		return &ExitError{Code: exitCancelled}
	}

	switch result.State {
	case core.StateComplete:
		return nil
	case core.StateBlocked:
		return &ExitError{Code: exitBlocked, Err: result.Error}
	case core.StateCancelled:
		return &ExitError{Code: exitCancelled, Err: result.Error}
	case core.StateFailed:
		if result.Error != nil {
			if errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, core.ErrLoopTimeout) {
				return &ExitError{Code: exitTimeout, Err: result.Error}
			}
			if errors.Is(result.Error, core.ErrMaxIterations) {
				return &ExitError{Code: exitMaxIterations, Err: result.Error}
			}
		}
		return &ExitError{Code: exitFailed, Err: result.Error}
	default:
		return &ExitError{Code: exitFailed}
	}
}

// resolvePrompt determines the prompt from the positional argument.
// If the argument is "-", the prompt is read from stdin.
// If the argument is a path to a Markdown file (.md/.markdown), its contents
// are returned. Otherwise the argument is returned verbatim.
func resolvePrompt(prompt string) (string, error) {
	if prompt == "" {
		return "", errors.New("no prompt provided")
	}

	if prompt == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading prompt from stdin: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	info, err := os.Stat(prompt)
	if err != nil {
		// File does not exist (or is unreadable); treat the argument as a literal prompt string.
		return prompt, nil //nolint:nilerr // intentional: argument is a literal prompt when not a file path
	}

	if info.IsDir() {
		return "", fmt.Errorf("prompt path %s is a directory, must be a Markdown file", prompt)
	}

	ext := strings.ToLower(filepath.Ext(prompt))
	if ext != ".md" && ext != ".markdown" {
		return "", fmt.Errorf("file %s must be a Markdown file with extension .md or .markdown", prompt)
	}

	data, err := os.ReadFile(prompt)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file %s: %w", prompt, err)
	}

	return string(data), nil
}

// buildLoopConfig creates a LoopConfig from command-line flags. It returns
// an error when --prompt-stack entries cannot be resolved.
func buildLoopConfig(prompt string) (*core.LoopConfig, error) {
	stack, err := resolvePromptStack(runPromptStack)
	if err != nil {
		return nil, err
	}

	return &core.LoopConfig{
		Prompt:               prompt,
		MaxIterations:        runMaxIterations,
		Timeout:              runTimeout,
		IterationTimeout:     runIterTimeout,
		PromisePhrase:        runPromise,
		Model:                runModel,
		WorkingDir:           runWorkingDir,
		DryRun:               runDryRun,
		Streaming:            runStreaming,
		LogLevel:             runLogLevel,
		SystemPrompt:         runSystemPrompt,
		SystemPromptMode:     runSystemPromptMode,
		CarryContext:         core.CarryContextMode(runCarryContext),
		CarryContextMaxRunes: runCarryMaxRunes,
		PlanFile:             runPlanFile,
		SpecsDir:             runSpecsDir,
		StopOnNoChanges:      runStopOnNoChanges,
		StopOnError:          runStopOnError,
		VerifyCmd:            runVerifyCmd,
		VerifyTimeout:        runVerifyTimeout,
		VerifyMaxBytes:       runVerifyMaxBytes,
		AutoCommit:           runAutoCommit,
		AutoCommitMessage:    runAutoCommitMsg,
		// flag is "force"; config field is the inverse (gate by verify success).
		AutoCommitOnVerifyOnly: !runAutoCommitForce,
		AutoTag:                runAutoTag,
		EmitDiffStat:           runDiffStat,
		CheckpointFile:         runCheckpoint,
		OracleModel:            runOracleModel,
		OracleEvery:            runOracleEvery,
		OracleOnVerifyFail:     runOracleOnVerifyErr,
		PromptStack:            stack,
		NoRateLimitWait:        runNoRateLimitWait,
		BlockedPhrase:          runBlockedPhrase,
		StallAfter:             runStallAfter,
		IterationDelay:         runIterationDelay,
		OnCompleteCmd:          runOnComplete,
		OnBlockedCmd:           runOnBlocked,
	}, nil
}

// resolvePromptStack resolves each entry in the --prompt-stack flag. Each
// entry is treated as a Markdown file path when it points at an existing
// .md/.markdown file; otherwise it is taken literally. Empty entries are
// skipped.
func resolvePromptStack(entries []string) ([]string, error) {
	out := make([]string, 0, len(entries))
	for _, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		text, err := resolvePrompt(raw)
		if err != nil {
			return nil, fmt.Errorf("--prompt-stack entry %q: %w", raw, err)
		}
		out = append(out, text)
	}
	return out, nil
}

// validateRunConfig validates the loop configuration.
func validateRunConfig(cfg *core.LoopConfig) error {
	if cfg.Prompt == "" {
		return errors.New("prompt cannot be empty")
	}

	if cfg.MaxIterations <= 0 {
		return fmt.Errorf("max-iterations must be positive (got: %d)", cfg.MaxIterations)
	}

	if cfg.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive (got: %v)", cfg.Timeout)
	}

	return nil
}

// validateSettings validates additional CLI flag settings on the resolved config.
func validateSettings(cfg *core.LoopConfig) error {
	if cfg.SystemPromptMode != "append" && cfg.SystemPromptMode != "replace" {
		return fmt.Errorf("invalid system-prompt-mode: %q (must be append or replace)", cfg.SystemPromptMode)
	}

	switch cfg.CarryContext {
	case "", core.CarryContextOff, core.CarryContextSummary, core.CarryContextVerbatim:
	default:
		return fmt.Errorf("invalid carry-context: %q (must be off, summary, or verbatim)", cfg.CarryContext)
	}

	if cfg.IterationTimeout < 0 {
		return fmt.Errorf("iteration-timeout must be >= 0 (got: %v)", cfg.IterationTimeout)
	}

	if cfg.IterationTimeout > 0 && cfg.Timeout > 0 && cfg.IterationTimeout > cfg.Timeout {
		return fmt.Errorf("iteration-timeout (%v) cannot exceed timeout (%v)", cfg.IterationTimeout, cfg.Timeout)
	}

	if cfg.StopOnNoChanges < 0 {
		return fmt.Errorf("stop-on-no-changes must be >= 0 (got: %d)", cfg.StopOnNoChanges)
	}

	if cfg.StopOnError < 0 {
		return fmt.Errorf("stop-on-error must be >= 0 (got: %d)", cfg.StopOnError)
	}

	if cfg.StallAfter < 0 {
		return fmt.Errorf("stall-after must be >= 0 (got: %d)", cfg.StallAfter)
	}

	if cfg.IterationDelay < 0 {
		return fmt.Errorf("iteration-delay must be >= 0 (got: %v)", cfg.IterationDelay)
	}

	return nil
}

// printDryRun displays what would be executed without running.
func printDryRun(cfg *core.LoopConfig) error { //nolint:unparam // signature kept consistent with RunE error contract
	fmt.Println(styles.TitleStyle.Render("🔍 Dry Run - Configuration Preview"))
	fmt.Println()
	fmt.Println(styles.InfoStyle.Render("  Prompt:            ") + cfg.Prompt)
	fmt.Println(styles.InfoStyle.Render("  Model:             ") + cfg.Model)
	fmt.Println(styles.InfoStyle.Render("  Max iterations:    ") + fmt.Sprintf("%d", cfg.MaxIterations))
	fmt.Println(styles.InfoStyle.Render("  Timeout:           ") + cfg.Timeout.String())
	fmt.Println(styles.InfoStyle.Render("  Promise phrase:    ") + cfg.PromisePhrase)
	fmt.Println(styles.InfoStyle.Render("  Working directory: ") + cfg.WorkingDir)
	fmt.Println()
	return nil
}

// printLoopConfig displays the loop configuration before starting.
func printLoopConfig(cfg *core.LoopConfig) {
	// Print Ralph ASCII art
	fmt.Println(styles.InfoStyle.Render(styles.RalphFox))
	fmt.Println()

	fmt.Println(styles.TitleStyle.Render("▶ Starting Ralph Loop"))
	fmt.Println(styles.WarningStyle.Render("Prompt:         ") + cfg.Prompt)
	fmt.Println(styles.WarningStyle.Render("Model:          ") + cfg.Model)
	fmt.Println(styles.WarningStyle.Render("Max iterations: ") + fmt.Sprintf("%d", cfg.MaxIterations))
	fmt.Println(styles.WarningStyle.Render("Timeout:        ") + cfg.Timeout.String())
	fmt.Println(styles.WarningStyle.Render("Working dir:    ") + cfg.WorkingDir)
}

// displayEventsWithSinks fans every event out to the configured
// sinks and (unless --json is set) renders a styled console view via
// displayEvents. When --json is set, sinks are responsible for output.
func displayEventsWithSinks(events <-chan any, cfg *core.LoopConfig, sinks *eventsink.FanOut) {
	if runJSON {
		// Pure JSON mode: sinks already include a stdout JSON sink.
		for ev := range events {
			if sinks != nil {
				sinks.Write(ev)
			}
			if _, ok := ev.(*core.LoopCompleteEvent); ok {
				return
			}
			if _, ok := ev.(*core.LoopFailedEvent); ok {
				return
			}
			if _, ok := ev.(*core.LoopCancelledEvent); ok {
				return
			}
		}
		return
	}

	// Tee styled rendering and sink fan-out.
	relay := make(chan any, 100)
	go func() {
		defer close(relay)
		for ev := range events {
			if sinks != nil {
				sinks.Write(ev)
			}
			relay <- ev
		}
	}()
	displayEvents(relay, cfg)
}

// buildEventSinks assembles the FanOut from --json-output / --log-file /
// --webhook flags. It also installs a stdout JSON sink when --json is set
// so the loop output is machine-readable.
func buildEventSinks() (*eventsink.FanOut, error) {
	fan := &eventsink.FanOut{}

	if runJSON {
		fan.Add(eventsink.NewJSONSink(os.Stdout))
	}

	if runJSONOutput != "" {
		s, err := eventsink.NewJSONFileSink(runJSONOutput)
		if err != nil {
			return nil, err
		}
		fan.Add(s)
	}

	if runLogFile != "" {
		s, err := eventsink.NewLogFileSink(runLogFile)
		if err != nil {
			return nil, err
		}
		fan.Add(s)
	}

	if runWebhook != "" {
		fan.Add(eventsink.NewWebhookSink(runWebhook, runWebhookTimeout))
	}

	return fan, nil
}

// displayEvents listens for loop events and displays them to stdout.
func displayEvents(events <-chan any, cfg *core.LoopConfig) {
	var newline bool
	for event := range events {
		var terminal bool
		terminal, newline = displayEvent(event, cfg, newline)
		if terminal {
			return
		}
	}
}

// displayEvent renders a single loop event to stdout and reports whether
// the caller should stop consuming events (terminal) and the updated
// newline state (updatedNewline is true when the output left the cursor
// without a trailing newline, as happens after an AIResponseEvent).
// newline indicates whether the previous output left the cursor without a
// trailing newline.
func displayEvent(event any, cfg *core.LoopConfig, newline bool) (terminal bool, updatedNewline bool) {
	switch e := event.(type) {
	case *core.LoopStartEvent:
		fmt.Println()
		fmt.Print(styles.TitleStyle.Render("▶ Loop started"))

	case *core.IterationStartEvent:
		fmt.Println()
		fmt.Println(styles.SubTitleStyle.Render(fmt.Sprintf("━━━ Iteration %d/%d ━━━", e.Iteration, cfg.MaxIterations)))
		fmt.Println()

	case *core.AIResponseEvent:
		// Print as we receive it for streaming effect
		fmt.Print(e.Text)

	case *core.ToolExecutionStartEvent:
		// Print newline if previous event was AI response
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.InfoStyle.Render(e.Info("🛠️")))

	case *core.ToolExecutionEvent:
		if e.Error != nil {
			errStr := styles.ErrorStyle.Render(fmt.Sprintf("(%s)", e.Error))
			fmt.Printf("%s %s\n", e.Info("❌"), errStr)
			// preserve newline state (matches original `continue` semantics)
			return false, newline
		}
		fmt.Println(styles.SuccessStyle.Render(e.Info("✔️")))

	case *core.IterationCompleteEvent:
		// Print newline if previous event was AI response
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.InfoStyle.Render(fmt.Sprintf("✓ Iteration %d complete", e.Iteration)))

	case *core.PromiseDetectedEvent:
		// Print newline if previous event was AI response
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.SuccessStyle.Render(fmt.Sprintf("🎉 Promise detected: \"%s\"", e.Phrase)))

	case *core.ErrorEvent:
		// Print newline if previous event was AI response
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.ErrorStyle.Render(fmt.Sprintf("✗ Error: %v", e.Error)))

	case *core.RateLimitWaitEvent:
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.WarningStyle.Render(formatRateLimitWait(e)))

	case *core.PlanUpdatedEvent:
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.InfoStyle.Render(fmt.Sprintf("📝 Plan updated: %s (%d bytes)", e.Path, e.Bytes)))

	case *core.IterationTimeoutEvent:
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.WarningStyle.Render(fmt.Sprintf("⏱ Iteration %d hit per-iteration timeout (%s)", e.Iteration, e.Timeout)))

	case *core.NoChangesStopEvent:
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.WarningStyle.Render(fmt.Sprintf("⛔ Stopping: %d consecutive iterations with no changes", e.Threshold)))

	case *core.ErrorStopEvent:
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.ErrorStyle.Render(fmt.Sprintf("⛔ Stopping: %d consecutive iterations with errors", e.Threshold)))

	case *core.BlockedPhraseDetectedEvent:
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.WarningStyle.Render(fmt.Sprintf("⛔ Blocked: model signalled it cannot proceed (phrase: %q)", e.Phrase)))

	case *core.StallStopEvent:
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.WarningStyle.Render(fmt.Sprintf("⛔ Stopping: %d consecutive identical responses (stall detected)", e.Threshold)))

	case *core.VerifyResultEvent:
		if newline {
			fmt.Println()
		}
		if e.Success {
			fmt.Println(styles.SuccessStyle.Render(fmt.Sprintf("✅ verify passed (%s)", e.Duration.Round(time.Millisecond))))
		}
		if !e.Success {
			header := fmt.Sprintf("❌ verify failed (exit %d, %s)", e.ExitCode, e.Duration.Round(time.Millisecond))
			if e.TimedOut {
				header = fmt.Sprintf("❌ verify timed out (%s)", e.Duration.Round(time.Millisecond))
			}
			fmt.Println(styles.ErrorStyle.Render(header))
			if strings.TrimSpace(e.Output) != "" {
				fmt.Println(styles.InfoStyle.Render(strings.TrimRight(e.Output, "\n")))
			}
		}

	case *core.AutoCommitEvent:
		if newline {
			fmt.Println()
		}
		tagSuffix := ""
		if e.Tag != "" {
			tagSuffix = fmt.Sprintf(" tag=%s", e.Tag)
		}
		fmt.Println(styles.SuccessStyle.Render(fmt.Sprintf("📦 auto-committed %s: %s%s", e.SHA, e.Message, tagSuffix)))

	case *core.WorkspaceDiffEvent:
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.InfoStyle.Render("📈 diff --stat:"))
		fmt.Println(e.Stat)

	case *core.OracleAdviceEvent:
		if newline {
			fmt.Println()
		}
		header := fmt.Sprintf("🔮 oracle (%s)", e.Model)
		if e.Reason != "" {
			header = fmt.Sprintf("%s — %s", header, e.Reason)
		}
		fmt.Println(styles.InfoStyle.Render(header))
		fmt.Println(strings.TrimRight(e.Advice, "\n"))

	case *core.CheckpointSavedEvent:
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.InfoStyle.Render(fmt.Sprintf("💾 checkpoint saved: %s (iteration %d)", e.Path, e.Iteration)))

	case *core.LoopCompleteEvent:
		// Will be handled by summary
		return true, false

	case *core.LoopFailedEvent:
		// Will be handled by summary
		return true, false

	case *core.LoopCancelledEvent:
		// Print newline if previous event was AI response
		if newline {
			fmt.Println()
		}
		fmt.Println(styles.WarningStyle.Render("⚠ Loop cancelled"))
		return true, false
	}

	_, updatedNewline = event.(*core.AIResponseEvent)
	return false, updatedNewline
}

// printSummary displays the final loop summary.
func printSummary(result *core.LoopResult, startTime time.Time) {
	duration := time.Since(startTime)

	fmt.Println()
	fmt.Println(styles.TitleStyle.Render("📊 Loop Summary"))

	// Status with color
	var status string
	switch result.State {
	case core.StateComplete:
		status = styles.SuccessStyle.Render("✓ Complete")
	case core.StateBlocked:
		status = styles.WarningStyle.Render("⛔ Blocked")
	case core.StateFailed:
		status = styles.ErrorStyle.Render("✗ Failed")
	case core.StateCancelled:
		status = styles.WarningStyle.Render("⚠ Cancelled")
	default:
		status = result.State.String()
	}

	fmt.Println(styles.InfoStyle.Render("Status:     ") + status)
	fmt.Println(styles.InfoStyle.Render("Iterations: ") + fmt.Sprintf("%d", result.Iterations))
	fmt.Println(styles.InfoStyle.Render("Duration:   ") + duration.Round(time.Second).String())

	if result.Error != nil {
		fmt.Println(styles.ErrorStyle.Render("Error:      ") + result.Error.Error())
	}

	fmt.Println()
}

var newCopilotClient = func(opts ...sdk.ClientOption) (core.SDKClient, error) {
	return sdk.NewCopilotClient(opts...)
}

// createSDKClient creates an SDK client with the given configuration.
func createSDKClient(loopConfig *core.LoopConfig) (core.SDKClient, error) {
	opts := []sdk.ClientOption{
		sdk.WithModel(loopConfig.Model),
		sdk.WithWorkingDir(loopConfig.WorkingDir),
		sdk.WithTimeout(loopConfig.Timeout),
		sdk.WithStreaming(loopConfig.Streaming),
		sdk.WithLogLevel(loopConfig.LogLevel),
	}

	// Build system prompt from template with the user's promise phrase.
	systemPrompt := core.BuildSystemPrompt(loopConfig.PromisePhrase)

	if loopConfig.SystemPrompt != "" {
		custom, err := resolvePrompt(loopConfig.SystemPrompt)
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdk.WithSystemMessage(custom, loopConfig.SystemPromptMode))
	}
	if loopConfig.SystemPrompt == "" {
		opts = append(opts, sdk.WithSystemMessage(systemPrompt, "append"))
	}

	return newCopilotClient(opts...)
}

// formatRateLimitWait builds a user-facing message describing a rate-limit
// pause. It includes the reset time when known and always reports how long
// the loop will sleep before retrying.
func formatRateLimitWait(e *core.RateLimitWaitEvent) string {
	wait := e.Wait.Round(time.Second)
	if e.HasReset {
		reset := e.ResetAt.Local().Format("Jan 2 15:04 MST")
		return fmt.Sprintf("⏳ Copilot rate limit reached; resuming at %s (waiting %s)", reset, wait)
	}
	if e.Message != "" {
		return fmt.Sprintf("⏳ Copilot rate limit reached; waiting %s before retry (%s)", wait, e.Message)
	}
	return fmt.Sprintf("⏳ Copilot rate limit reached; waiting %s before retry", wait)
}

// runHook executes a shell command hook (--on-complete / --on-blocked) with a
// short timeout. Errors are printed as warnings; a failing hook never changes
// Ralph's own exit code.
func runHook(shellCmd, workingDir string, result *core.LoopResult) {
	if shellCmd == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", shellCmd) //nolint:gosec // shellCmd is user-supplied, intentional
	cmd.Dir = workingDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("RALPH_STATE=%s", result.State),
		fmt.Sprintf("RALPH_ITERATIONS=%d", result.Iterations),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, styles.WarningStyle.Render(
			fmt.Sprintf("⚠ hook %q failed: %v\n%s", shellCmd, err, strings.TrimRight(string(out), "\n"))))
	}
}

// envString returns the value of an environment variable, or def if it is unset or empty.
func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envInt returns the integer value of an environment variable, or def if it
// is unset, empty, or not parseable as an integer.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envDuration returns the duration value of an environment variable, or def
// if it is unset, empty, or not parseable.
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// envBool returns the boolean value of an environment variable, or def if it
// is unset, empty, or not parseable.
func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

// Package core implements the Ralph loop engine and business logic.
//
// This package contains the core loop execution engine that orchestrates
// iterative AI development loops. It manages state transitions, promise
// detection, and event emission.
//
// The LoopEngine follows a state machine pattern with the following states:
//   - StateIdle: Initial state, ready to start
//   - StateRunning: Loop is executing iterations
//   - StateComplete: Successfully completed (promise detected)
//   - StateFailed: Failed due to error, timeout, or max iterations
//   - StateCancelled: Cancelled by user
//
// See .github/copilot-instructions.md for the full developer guide and architectural overview.
package core

import (
	"context"
	"strings"
	"sync"
	"time"
)

// LoopState represents the current state of the loop.
type LoopState string

const (
	// StateIdle indicates the loop is ready to start.
	StateIdle LoopState = "idle"
	// StateRunning indicates the loop is executing iterations.
	StateRunning LoopState = "running"
	// StateComplete indicates the loop completed successfully.
	StateComplete LoopState = "complete"
	// StateFailed indicates the loop failed.
	StateFailed LoopState = "failed"
	// StateCancelled indicates the loop was cancelled.
	StateCancelled LoopState = "cancelled"
)

// String returns the string representation of the state.
func (s LoopState) String() string {
	return string(s)
}

// CarryContextMode controls how a previous iteration's reply is fed into
// the next iteration's prompt.
type CarryContextMode string

const (
	// CarryContextOff disables carry-context. Each iteration sees only the
	// original task prompt. This was Ralph's pre-Phase-A behavior.
	CarryContextOff CarryContextMode = "off"
	// CarryContextSummary feeds forward only the contents of the last
	// `<summary>...</summary>` block emitted by the assistant. This is the
	// default and matches Huntley's "summarise then carry forward" guidance.
	CarryContextSummary CarryContextMode = "summary"
	// CarryContextVerbatim feeds the full last assistant reply forward. It
	// is the most expensive option and exists for debugging.
	CarryContextVerbatim CarryContextMode = "verbatim"
)

// LoopConfig contains configuration for loop execution.
type LoopConfig struct {
	Prompt           string
	PromisePhrase    string
	Model            string
	WorkingDir       string
	LogLevel         string
	SystemPrompt     string
	SystemPromptMode string

	// CarryContext controls how the previous iteration's reply is fed into
	// the next iteration's prompt. Defaults to CarryContextSummary.
	CarryContext CarryContextMode
	// CarryContextMaxRunes clamps a carried summary so a chatty assistant
	// cannot inflate the next prompt indefinitely. <=0 disables truncation.
	CarryContextMaxRunes int

	// PlanFile is the path (relative or absolute) to the running fix_plan.md
	// scratchpad. Empty means "do not surface a plan file in the prompt".
	PlanFile string
	// SpecsDir is a directory whose Markdown files are listed in the prompt
	// so the assistant knows specs exist. Empty means "no specs".
	SpecsDir string

	// StopOnNoChanges halts the loop when this many consecutive iterations
	// leave the git working tree unchanged. <=0 disables the check.
	StopOnNoChanges int
	// StopOnError halts the loop when this many consecutive iterations emit
	// an error event. <=0 disables the check.
	StopOnError int

	// IterationTimeout is the per-iteration soft deadline. <=0 disables.
	IterationTimeout time.Duration

	// VerifyCmd is a shell command run after each iteration. When it
	// exits non-zero its captured output is folded into the next
	// iteration's prompt so the assistant can react to failing
	// tests/builds. Empty disables the feature.
	VerifyCmd string
	// VerifyTimeout caps a single verify run. <=0 disables.
	VerifyTimeout time.Duration
	// VerifyMaxBytes truncates the captured stdout/stderr/combined buffers
	// per stream. <=0 means unlimited (use with care).
	VerifyMaxBytes int

	// AutoCommit, when true, runs `git add -A && git commit` after every
	// iteration that produced changes. Requires WorkingDir to be a git
	// repository.
	AutoCommit bool
	// AutoCommitOnVerifyOnly, when true, only auto-commits when verify
	// succeeded (or is disabled). Defaults to true to avoid recording
	// known-bad states.
	AutoCommitOnVerifyOnly bool
	// AutoCommitMessage is a Go template-free format string for the commit
	// message. Use %d for the iteration number. Empty falls back to a
	// sensible default.
	AutoCommitMessage string
	// AutoTag, when non-empty, creates an annotated tag (with %d for
	// iteration number) on each successful auto-commit. Tags are never
	// pushed.
	AutoTag string
	// EmitDiffStat, when true, emits a WorkspaceDiffEvent at the end of
	// each iteration carrying `git diff --stat HEAD`.
	EmitDiffStat bool

	// CheckpointFile is the path the engine writes loop state to after
	// each iteration. Empty disables checkpointing.
	CheckpointFile string
	// ResumeFromIteration, when >0, instructs the engine to start
	// numbering iterations from this value+1 (i.e. it continues a
	// previously-checkpointed run). The engine treats it as advisory and
	// still respects MaxIterations.
	ResumeFromIteration int
	// ResumeSummary, when non-empty, primes the carry-context summary so
	// the first resumed iteration sees the previous run's last summary.
	ResumeSummary string

	// OracleModel is the model used for the second-opinion oracle. Empty
	// disables oracle consultations entirely.
	OracleModel string
	// OracleEvery, when >0, consults the oracle every N iterations.
	OracleEvery int
	// OracleOnVerifyFail, when true, also consults the oracle whenever
	// --verify-cmd fails for an iteration.
	OracleOnVerifyFail bool

	// PromptStack lists additional prompts (already resolved to text)
	// that are prepended to the main prompt in order.
	PromptStack []string

	// NoRateLimitWait, when true, surfaces Copilot rate-limit errors as
	// fatal instead of pausing the loop until reset.
	NoRateLimitWait bool

	MaxIterations int
	Timeout       time.Duration
	DryRun        bool
	Streaming     bool
}

// DefaultLoopConfig returns a LoopConfig with default values.
func DefaultLoopConfig() *LoopConfig {
	return &LoopConfig{
		MaxIterations:          10,
		Timeout:                30 * time.Minute,
		PromisePhrase:          "I'm special!",
		Model:                  "gpt-4",
		WorkingDir:             ".",
		LogLevel:               "info",
		SystemPromptMode:       "append",
		Streaming:              true,
		CarryContext:           CarryContextSummary,
		CarryContextMaxRunes:   4000,
		VerifyMaxBytes:         16 * 1024,
		AutoCommitOnVerifyOnly: true,
	}
}

// LoopEngine manages the execution of AI development loops.
// It coordinates with the Copilot SDK, detects completion promises,
// and handles state transitions.
type LoopEngine struct {
	startTime    time.Time
	sdk          SDKClient
	ctx          context.Context
	config       *LoopConfig
	events       chan any
	cancel       context.CancelFunc
	state        LoopState
	iteration    int
	mu           sync.RWMutex
	eventsClosed bool

	// lastSummary holds the carry-context payload produced by the previous
	// iteration. For CarryContextSummary it is the contents of the last
	// `<summary>` block; for CarryContextVerbatim it is the raw response.
	lastSummary string
	// consecutiveNoChanges counts iterations in a row whose working tree
	// stayed clean (only meaningful when StopOnNoChanges > 0).
	consecutiveNoChanges int
	// consecutiveErrors counts iterations in a row that emitted at least
	// one error event (only meaningful when StopOnError > 0).
	consecutiveErrors int

	// lastVerify holds the previous iteration's verify command result.
	// When non-nil and unsuccessful it is folded into the next iteration's
	// prompt so the assistant sees the failure.
	lastVerify *verifyResultSummary

	// lastOracleAdvice holds the most recent oracle advice. It is folded
	// into the next iteration's prompt and cleared after consumption.
	lastOracleAdvice string

	// oracle is the optional second-opinion client. nil disables.
	oracle OracleClient
}

// verifyResultSummary is a tiny carry-context payload describing the
// previous iteration's verify run. It is built from a verify.Result but
// kept here so the core package does not import verify in its public API.
type verifyResultSummary struct {
	Cmd      string
	Output   string
	ExitCode int
	Success  bool
	TimedOut bool
}

// eventChannelBufferSize is the buffer size for the events channel.
const eventChannelBufferSize = 100

// NewLoopEngine creates a new loop engine with the given configuration.
// If sdk is nil, the engine will run in dry-run mode.
func NewLoopEngine(config *LoopConfig, sdk SDKClient) *LoopEngine {
	if config == nil {
		config = DefaultLoopConfig()
	}

	e := &LoopEngine{
		config: config,
		sdk:    sdk,
		state:  StateIdle,
		events: make(chan any, eventChannelBufferSize),
	}
	if config.ResumeSummary != "" {
		e.lastSummary = config.ResumeSummary
	}
	if config.ResumeFromIteration > 0 {
		e.iteration = config.ResumeFromIteration
	}
	return e
}

// OracleClient produces a single advisory response from a second-opinion
// model. Implementations must respect the supplied context and return
// the assistant's text in full.
type OracleClient interface {
	Consult(ctx context.Context, prompt string) (string, error)
}

// SetOracle attaches an OracleClient. Calling with nil disables oracle
// consultations even when OracleEvery > 0.
func (e *LoopEngine) SetOracle(o OracleClient) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.oracle = o
}

// State returns the current loop state.
func (e *LoopEngine) State() LoopState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// BuildSystemPrompt constructs the system prompt from the embedded template.
// It replaces {{.Task}} with the actual user task and {{.Promise}} with the completion phrase.
func BuildSystemPrompt(promisePhrase string) string {
	return strings.ReplaceAll(systemPromptTemplate, "{{.Promise}}", promisePhrase)
}

// Iteration returns the current iteration number (1-based).
// Returns 0 if the loop hasn't started yet.
func (e *LoopEngine) Iteration() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.iteration
}

// Config returns the loop configuration.
func (e *LoopEngine) Config() *LoopConfig {
	return e.config
}

// Events returns a read-only channel for receiving loop events.
// Subscribers should read from this channel to receive updates.
func (e *LoopEngine) Events() <-chan any {
	return e.events
}

// LoopResult contains the outcome of loop execution.
type LoopResult struct {
	Error      error
	State      LoopState
	Iterations int
	Duration   time.Duration
}

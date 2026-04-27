// Package core provides loop event types for the loop engine.
package core

import (
	"fmt"
	"strings"
	"time"
)

// LoopStartEvent indicates the loop has started.
type LoopStartEvent struct {
	// Config is the loop configuration.
	Config *LoopConfig
}

// NewLoopStartEvent creates a new LoopStartEvent.
func NewLoopStartEvent(config *LoopConfig) *LoopStartEvent {
	return &LoopStartEvent{
		Config: config,
	}
}

// LoopCompleteEvent indicates the loop completed successfully.
type LoopCompleteEvent struct {
	// Result contains the loop result.
	Result *LoopResult
}

// NewLoopCompleteEvent creates a new LoopCompleteEvent.
func NewLoopCompleteEvent(result *LoopResult) *LoopCompleteEvent {
	return &LoopCompleteEvent{
		Result: result,
	}
}

// LoopFailedEvent indicates the loop failed.
type LoopFailedEvent struct {
	// Error is the error that caused the failure.
	Error error
	// Result contains partial loop result.
	Result *LoopResult
}

// NewLoopFailedEvent creates a new LoopFailedEvent.
func NewLoopFailedEvent(err error, result *LoopResult) *LoopFailedEvent {
	return &LoopFailedEvent{
		Error:  err,
		Result: result,
	}
}

// LoopCancelledEvent indicates the loop was cancelled by the user.
type LoopCancelledEvent struct {
	// Result contains partial loop result.
	Result *LoopResult
}

// NewLoopCancelledEvent creates a new LoopCancelledEvent.
func NewLoopCancelledEvent(result *LoopResult) *LoopCancelledEvent {
	return &LoopCancelledEvent{
		Result: result,
	}
}

// IterationStartEvent indicates an iteration has started.
type IterationStartEvent struct {
	// Iteration is the iteration number (1-based).
	Iteration int
	// MaxIterations is the maximum number of iterations.
	MaxIterations int
}

// NewIterationStartEvent creates a new IterationStartEvent.
func NewIterationStartEvent(iteration, maxIterations int) *IterationStartEvent {
	return &IterationStartEvent{
		Iteration:     iteration,
		MaxIterations: maxIterations,
	}
}

// IterationCompleteEvent indicates an iteration completed.
type IterationCompleteEvent struct {
	// Iteration is the iteration number (1-based).
	Iteration int
	// Duration is how long the iteration took.
	Duration time.Duration
}

// NewIterationCompleteEvent creates a new IterationCompleteEvent.
func NewIterationCompleteEvent(iteration int, duration time.Duration) *IterationCompleteEvent {
	return &IterationCompleteEvent{
		Iteration: iteration,
		Duration:  duration,
	}
}

// AIResponseEvent indicates AI response text was received.
type AIResponseEvent struct {
	// Text is the AI response text.
	Text string
	// Iteration is the current iteration number.
	Iteration int
}

// NewAIResponseEvent creates a new AIResponseEvent.
func NewAIResponseEvent(text string, iteration int) *AIResponseEvent {
	return &AIResponseEvent{
		Text:      text,
		Iteration: iteration,
	}
}

// ToolEvent describes a tool invocation request observed by the loop engine.
type ToolEvent struct {
	Parameters map[string]any
	ToolName   string
	Iteration  int
}

// Info returns a formatted string describing the tool execution based on parameters.
// This provides human-readable information about what the tool is doing.
func (e *ToolEvent) Info(emoji string) string {
	if len(e.Parameters) == 0 {
		return fmt.Sprintf("%s %s", emoji, e.ToolName)
	}

	var values []string
	for _, v := range e.Parameters {
		values = append(values, fmt.Sprintf("%v", v))
	}

	return fmt.Sprintf("%s %s: %s", emoji, e.ToolName, strings.Join(values, ", "))
}

// ToolExecutionEvent indicates a tool was executed.
type ToolExecutionEvent struct {
	Error error
	ToolEvent
	Result   string
	Duration time.Duration
}

// NewToolExecutionEvent creates a new ToolExecutionEvent.
func NewToolExecutionEvent(toolName string, params map[string]any, result string, err error, duration time.Duration, iteration int) *ToolExecutionEvent {
	return &ToolExecutionEvent{
		ToolEvent: ToolEvent{
			ToolName:   toolName,
			Parameters: params,
			Iteration:  iteration,
		},
		Result:   result,
		Error:    err,
		Duration: duration,
	}
}

// ToolExecutionStartEvent indicates a tool execution has started.
type ToolExecutionStartEvent struct {
	ToolEvent
}

// NewToolExecutionStartEvent creates a new ToolExecutionStartEvent.
func NewToolExecutionStartEvent(toolName string, params map[string]any, iteration int) *ToolExecutionStartEvent {
	return &ToolExecutionStartEvent{
		ToolEvent: ToolEvent{
			ToolName:   toolName,
			Parameters: params,
			Iteration:  iteration,
		},
	}
}

// PromiseDetectedEvent indicates the promise phrase was found.
type PromiseDetectedEvent struct {
	// Phrase is the promise phrase that was detected.
	Phrase string
	// Source is where the promise was found (e.g., "ai_response", "tool_output").
	Source string
	// Iteration is the iteration number where promise was found.
	Iteration int
}

// NewPromiseDetectedEvent creates a new PromiseDetectedEvent.
func NewPromiseDetectedEvent(phrase, source string, iteration int) *PromiseDetectedEvent {
	return &PromiseDetectedEvent{
		Phrase:    phrase,
		Source:    source,
		Iteration: iteration,
	}
}

// ErrorEvent indicates an error occurred.
type ErrorEvent struct {
	// Error is the error that occurred.
	Error error
	// Iteration is the current iteration number (0 if not in iteration).
	Iteration int
	// Recoverable indicates if the error is recoverable.
	Recoverable bool
}

// NewErrorEvent creates a new ErrorEvent.
func NewErrorEvent(err error, iteration int, recoverable bool) *ErrorEvent {
	return &ErrorEvent{
		Error:       err,
		Iteration:   iteration,
		Recoverable: recoverable,
	}
}

// RateLimitWaitEvent indicates the loop is paused waiting for a Copilot
// rate-limit / quota window to reset before retrying the current iteration.
type RateLimitWaitEvent struct {
	// ResetAt is when the rate limit is expected to reset. Only meaningful
	// when HasReset is true.
	ResetAt time.Time
	// Wait is the duration the engine will sleep before retrying.
	Wait time.Duration
	// Message is the human-readable message reported by the SDK.
	Message string
	// ErrorType is the SDK-reported error category (e.g. "rate_limit").
	ErrorType string
	// Iteration is the iteration number being retried.
	Iteration int
	// HasReset indicates whether ResetAt is meaningful.
	HasReset bool
}

// NewRateLimitWaitEvent creates a new RateLimitWaitEvent.
func NewRateLimitWaitEvent(message, errorType string, resetAt time.Time, hasReset bool, wait time.Duration, iteration int) *RateLimitWaitEvent {
	return &RateLimitWaitEvent{
		Message:   message,
		ErrorType: errorType,
		ResetAt:   resetAt,
		HasReset:  hasReset,
		Wait:      wait,
		Iteration: iteration,
	}
}

// PlanUpdatedEvent indicates the assistant changed the running fix_plan.md
// scratchpad during an iteration.
type PlanUpdatedEvent struct {
	// Path is the absolute path of the plan file.
	Path string
	// Bytes is the new size of the plan file on disk.
	Bytes int
	// Iteration is the iteration number that produced the change.
	Iteration int
}

// NewPlanUpdatedEvent creates a new PlanUpdatedEvent.
func NewPlanUpdatedEvent(path string, bytes, iteration int) *PlanUpdatedEvent {
	return &PlanUpdatedEvent{Path: path, Bytes: bytes, Iteration: iteration}
}

// NoChangesStopEvent indicates the loop is stopping because the working tree
// stayed clean for a configured number of consecutive iterations.
type NoChangesStopEvent struct {
	// Threshold is the configured number of clean iterations that triggers
	// the stop.
	Threshold int
	// Iteration is the iteration where the threshold was reached.
	Iteration int
}

// NewNoChangesStopEvent creates a new NoChangesStopEvent.
func NewNoChangesStopEvent(threshold, iteration int) *NoChangesStopEvent {
	return &NoChangesStopEvent{Threshold: threshold, Iteration: iteration}
}

// ErrorStopEvent indicates the loop is stopping because too many
// consecutive iterations emitted errors.
type ErrorStopEvent struct {
	// Threshold is the configured consecutive-error budget.
	Threshold int
	// Iteration is the iteration where the threshold was reached.
	Iteration int
}

// NewErrorStopEvent creates a new ErrorStopEvent.
func NewErrorStopEvent(threshold, iteration int) *ErrorStopEvent {
	return &ErrorStopEvent{Threshold: threshold, Iteration: iteration}
}

// IterationTimeoutEvent indicates an iteration hit its per-iteration
// soft deadline and was cut short. The loop continues with the next
// iteration unless other stop conditions fire.
type IterationTimeoutEvent struct {
	// Timeout is the configured per-iteration timeout.
	Timeout time.Duration
	// Iteration is the iteration that timed out.
	Iteration int
}

// NewIterationTimeoutEvent creates a new IterationTimeoutEvent.
func NewIterationTimeoutEvent(timeout time.Duration, iteration int) *IterationTimeoutEvent {
	return &IterationTimeoutEvent{Timeout: timeout, Iteration: iteration}
}

// VerifyResultEvent indicates the post-iteration verify command finished.
// It is emitted exactly once per iteration when --verify-cmd is set.
type VerifyResultEvent struct {
	Cmd       string
	Output    string
	Iteration int
	ExitCode  int
	Duration  time.Duration
	Success   bool
	TimedOut  bool
}

// NewVerifyResultEvent creates a new VerifyResultEvent.
func NewVerifyResultEvent(cmd, output string, exitCode, iteration int, duration time.Duration, success, timedOut bool) *VerifyResultEvent {
	return &VerifyResultEvent{
		Cmd:       cmd,
		Output:    output,
		ExitCode:  exitCode,
		Iteration: iteration,
		Duration:  duration,
		Success:   success,
		TimedOut:  timedOut,
	}
}

// WorkspaceDiffEvent reports a `git diff --stat HEAD` snapshot taken at
// the end of an iteration.
type WorkspaceDiffEvent struct {
	Stat      string
	Iteration int
}

// NewWorkspaceDiffEvent creates a new WorkspaceDiffEvent.
func NewWorkspaceDiffEvent(stat string, iteration int) *WorkspaceDiffEvent {
	return &WorkspaceDiffEvent{Stat: stat, Iteration: iteration}
}

// AutoCommitEvent indicates Ralph auto-committed the iteration's changes.
type AutoCommitEvent struct {
	SHA       string
	Message   string
	Tag       string
	Iteration int
}

// NewAutoCommitEvent creates a new AutoCommitEvent.
func NewAutoCommitEvent(sha, message, tag string, iteration int) *AutoCommitEvent {
	return &AutoCommitEvent{SHA: sha, Message: message, Tag: tag, Iteration: iteration}
}

// OracleAdviceEvent indicates the second-opinion oracle returned advice
// that will be folded into the next iteration's prompt.
type OracleAdviceEvent struct {
	Model     string
	Advice    string
	Iteration int
	Reason    string
}

// NewOracleAdviceEvent creates a new OracleAdviceEvent.
func NewOracleAdviceEvent(model, advice, reason string, iteration int) *OracleAdviceEvent {
	return &OracleAdviceEvent{Model: model, Advice: advice, Reason: reason, Iteration: iteration}
}

// CheckpointSavedEvent indicates the engine wrote a checkpoint file.
type CheckpointSavedEvent struct {
	Path      string
	Iteration int
}

// NewCheckpointSavedEvent creates a new CheckpointSavedEvent.
func NewCheckpointSavedEvent(path string, iteration int) *CheckpointSavedEvent {
	return &CheckpointSavedEvent{Path: path, Iteration: iteration}
}

// BlockedPhraseDetectedEvent indicates the assistant emitted the blocked
// signal, signalling it cannot make further progress without intervention.
type BlockedPhraseDetectedEvent struct {
	// Phrase is the blocked phrase that was detected.
	Phrase string
	// Iteration is the iteration where the phrase was detected.
	Iteration int
}

// NewBlockedPhraseDetectedEvent creates a new BlockedPhraseDetectedEvent.
func NewBlockedPhraseDetectedEvent(phrase string, iteration int) *BlockedPhraseDetectedEvent {
	return &BlockedPhraseDetectedEvent{Phrase: phrase, Iteration: iteration}
}

// StallStopEvent indicates the loop stopped because consecutive iterations
// produced identical assistant responses.
type StallStopEvent struct {
	// Threshold is the configured number of identical iterations that
	// triggers the stop.
	Threshold int
	// Iteration is the iteration where the threshold was reached.
	Iteration int
}

// NewStallStopEvent creates a new StallStopEvent.
func NewStallStopEvent(threshold, iteration int) *StallStopEvent {
	return &StallStopEvent{Threshold: threshold, Iteration: iteration}
}

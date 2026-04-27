// Package core provides the loop engine execution logic.
package core

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/patbaumgartner/copilot-ralph/internal/checkpoint"
	"github.com/patbaumgartner/copilot-ralph/internal/gitutil"
	"github.com/patbaumgartner/copilot-ralph/internal/planfile"
	"github.com/patbaumgartner/copilot-ralph/internal/sdk"
	"github.com/patbaumgartner/copilot-ralph/internal/specs"
	"github.com/patbaumgartner/copilot-ralph/internal/verify"
)

//go:embed system.md
var systemPromptTemplate string

// ErrLoopCancelled indicates the loop was cancelled by the user.
var ErrLoopCancelled = errors.New("loop cancelled")

// ErrLoopTimeout indicates the loop exceeded the configured timeout.
var ErrLoopTimeout = errors.New("loop timeout exceeded")

// ErrMaxIterations indicates the maximum iterations were reached.
var ErrMaxIterations = errors.New("maximum iterations reached")

// ErrNoChangesStop indicates the loop stopped because the working tree
// stayed clean for the configured number of consecutive iterations.
var ErrNoChangesStop = errors.New("loop stopped: no changes detected")

// ErrErrorStop indicates the loop stopped because too many consecutive
// iterations emitted errors.
var ErrErrorStop = errors.New("loop stopped: consecutive error threshold exceeded")

// ErrRateLimitFatal indicates the loop stopped because --no-rate-limit-wait
// is set and a rate-limit / quota error was reported.
var ErrRateLimitFatal = errors.New("loop stopped: rate limit reached and --no-rate-limit-wait is set")

// Start begins loop execution and runs until completion, failure, or cancellation.
// It returns the loop result containing statistics and outcome.
// The provided context can be used to cancel execution externally.
func (e *LoopEngine) Start(ctx context.Context) (*LoopResult, error) {
	e.mu.Lock()
	if e.state != StateIdle {
		e.mu.Unlock()
		return nil, errors.New("loop already running")
	}

	// Set up cancellation with timeout if configured
	if e.config.Timeout > 0 {
		e.ctx, e.cancel = context.WithTimeout(ctx, e.config.Timeout)
	}
	if e.config.Timeout <= 0 {
		e.ctx, e.cancel = context.WithCancel(ctx)
	}
	e.state = StateRunning
	e.startTime = time.Now()
	e.iteration = 0
	e.mu.Unlock()

	// Close events channel when engine finishes to unblock any listeners
	defer func() {
		e.mu.Lock()
		e.eventsClosed = true
		e.mu.Unlock()
		close(e.events)
	}()

	// Emit loop start event
	e.emit(NewLoopStartEvent(e.config))

	// Initialize SDK if provided
	if e.sdk != nil {
		if err := e.sdk.Start(); err != nil {
			return e.fail(fmt.Errorf("failed to start SDK: %w", err))
		}

		err := e.sdk.CreateSession(e.ctx)
		if err != nil {
			return e.fail(fmt.Errorf("failed to create SDK session: %w", err))
		}
	}

	// Run the main loop
	result, err := e.runLoop()

	// Clean up SDK - do it in background if cancelled for immediate return
	if e.sdk != nil {
		if result != nil && result.State == StateCancelled {
			// Background cleanup on cancellation - don't wait
			go func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cleanupCancel()
				_ = e.sdk.DestroySession(cleanupCtx)
				_ = e.sdk.Stop()
			}()
		}
		if result == nil || result.State != StateCancelled {
			// Normal cleanup - wait for completion
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = e.sdk.DestroySession(cleanupCtx)
			cleanupCancel()
			_ = e.sdk.Stop()
		}
	}

	return result, err
}

// runLoop executes the main iteration loop.
// The loop continues until all iterations are completed, timeout is hit, an
// error occurs, or a configured stop condition (no-changes / errors) fires.
// Promise detection is tracked but does not stop the loop.
func (e *LoopEngine) runLoop() (*LoopResult, error) {
	for {
		if result, err := e.preIterationCheck(); err != nil || result != nil {
			return result, err
		}

		// Execute iteration
		stop, err := e.executeIteration()
		if err != nil {
			// Check if it's a timeout (context deadline exceeded)
			if errors.Is(err, context.DeadlineExceeded) {
				return e.fail(ErrLoopTimeout)
			}
			// Check if it's a cancellation
			if errors.Is(err, context.Canceled) {
				return e.cancelled()
			}
			return e.fail(fmt.Errorf("iteration %d failed: %w", e.iteration, err))
		}

		if stop != nil {
			return e.fail(stop)
		}

		// Continue to next iteration (promise does not stop the loop)
	}
}

// preIterationCheck evaluates cancellation, state, and limit guards before running an iteration.
func (e *LoopEngine) preIterationCheck() (*LoopResult, error) {
	select {
	case <-e.ctx.Done():
		return e.cancelled()
	default:
	}

	e.mu.RLock()
	state := e.state
	e.mu.RUnlock()

	if state == StateCancelled {
		return e.cancelled()
	}

	if e.config.Timeout > 0 && time.Since(e.startTime) > e.config.Timeout {
		return e.fail(ErrLoopTimeout)
	}

	// Check if max iterations have been reached BEFORE starting a new one
	if e.config.MaxIterations > 0 && e.iteration >= e.config.MaxIterations {
		return e.complete()
	}

	return nil, nil
}

// executeIteration executes a single iteration of the loop. It returns a
// non-nil stop error when a configured stop condition (no-changes / errors)
// fires for this iteration; the second value is reserved for fatal errors
// that abort the run.
func (e *LoopEngine) executeIteration() (stop, fatal error) {
	e.mu.Lock()
	e.iteration++
	iteration := e.iteration
	e.mu.Unlock()

	iterationStart := time.Now()

	// Capture pre-iteration plan + git state for change detection.
	planPath := ""
	if e.config.PlanFile != "" {
		planPath = planfile.Resolve(e.config.WorkingDir, e.config.PlanFile)
	}
	prePlan, _ := e.snapshotPlan(planPath)
	preClean, gitAvailable := e.workingTreeClean()

	// Emit iteration start
	e.emit(NewIterationStartEvent(iteration, e.config.MaxIterations))

	// Per-iteration timeout context (soft deadline; never longer than the
	// outer loop deadline).
	iterCtx := e.ctx
	var cancelIter context.CancelFunc = func() {}
	if e.config.IterationTimeout > 0 {
		iterCtx, cancelIter = context.WithTimeout(e.ctx, e.config.IterationTimeout)
	}
	defer cancelIter()

	// Build context and send prompt
	prompt := e.buildIterationPrompt(iteration)

	var responseBuf strings.Builder
	iterErrors := 0

	if e.sdk != nil {
		events, err := e.sdk.SendPrompt(iterCtx, prompt)
		if err != nil {
			return nil, fmt.Errorf("failed to send prompt: %w", err)
		}

	eventLoop:
		for {
			select {
			case <-e.ctx.Done():
				return nil, e.ctx.Err()
			case <-iterCtx.Done():
				// Per-iteration soft timeout. We surface it as an event and
				// break out to let the loop continue with the next iteration.
				if e.ctx.Err() == nil && errors.Is(iterCtx.Err(), context.DeadlineExceeded) {
					e.emit(NewIterationTimeoutEvent(e.config.IterationTimeout, iteration))
					break eventLoop
				}
				return nil, iterCtx.Err()
			case event, ok := <-events:
				if !ok {
					break eventLoop
				}

				switch ev := event.(type) {
				case *sdk.TextEvent:
					if !ev.Reasoning {
						responseBuf.WriteString(ev.Text)
					}
					e.emit(NewAIResponseEvent(ev.Text, iteration))

					if !ev.Reasoning && detectPromise(ev.Text, e.config.PromisePhrase) {
						e.emit(NewPromiseDetectedEvent(e.config.PromisePhrase, "ai_response", iteration))
					}

				case *sdk.ToolCallEvent:
					e.emit(NewToolExecutionStartEvent(
						ev.ToolCall.Name,
						ev.ToolCall.Parameters,
						iteration,
					))

				case *sdk.ToolResultEvent:
					e.emit(NewToolExecutionEvent(
						ev.ToolCall.Name,
						ev.ToolCall.Parameters,
						ev.Result,
						ev.Error,
						0,
						iteration,
					))

				case *sdk.ErrorEvent:
					iterErrors++
					e.emit(NewErrorEvent(ev.Err, iteration, true))

				case *sdk.RateLimitEvent:
					e.emit(NewRateLimitWaitEvent(
						ev.Message,
						ev.ErrorType,
						ev.ResetAt,
						ev.HasReset,
						ev.Wait,
						iteration,
					))
					if e.config.NoRateLimitWait {
						return ErrRateLimitFatal, nil
					}
				}
			}
		}
	}

	// Update carry-context based on this iteration's response.
	e.updateCarryContext(responseBuf.String())

	// Detect plan changes and emit a single event when the plan moved.
	if planPath != "" {
		postPlan, _ := e.snapshotPlan(planPath)
		if planfile.Changed(prePlan, postPlan) {
			e.emit(NewPlanUpdatedEvent(planPath, len(postPlan.Content), iteration))
		}
	}

	// Run verify command (if any) and surface the result. A failing
	// verify is folded into the next iteration's prompt via lastVerify.
	verifySucceeded := e.runVerify(iteration)

	// Auto-commit and (optionally) auto-tag any changes the iteration
	// produced. We deliberately never push.
	e.autoCommit(iteration, verifySucceeded)

	// Emit a workspace diff snapshot when requested.
	if e.config.EmitDiffStat && gitAvailable {
		stat, err := gitutil.DiffStat(e.ctx, e.config.WorkingDir)
		if err == nil && strings.TrimSpace(stat) != "" {
			e.emit(NewWorkspaceDiffEvent(stat, iteration))
		}
	}

	// Update consecutive-error counter and check the StopOnError budget.
	if iterErrors > 0 {
		e.consecutiveErrors++
	}
	if iterErrors == 0 {
		e.consecutiveErrors = 0
	}
	if e.config.StopOnError > 0 && e.consecutiveErrors >= e.config.StopOnError {
		e.emit(NewErrorStopEvent(e.config.StopOnError, iteration))
		return ErrErrorStop, nil
	}

	// Update consecutive-no-changes counter and check the StopOnNoChanges
	// budget. When git is unavailable we skip the check entirely so the
	// feature degrades gracefully outside repositories.
	if gitAvailable && e.config.StopOnNoChanges > 0 {
		postClean, _ := e.workingTreeClean()
		if preClean && postClean {
			e.consecutiveNoChanges++
		}
		if !postClean {
			e.consecutiveNoChanges = 0
		}
		if e.consecutiveNoChanges >= e.config.StopOnNoChanges {
			e.emit(NewNoChangesStopEvent(e.config.StopOnNoChanges, iteration))
			return ErrNoChangesStop, nil
		}
	}

	iterationDuration := time.Since(iterationStart)
	e.emit(NewIterationCompleteEvent(iteration, iterationDuration))

	// Consult oracle (if any) once the iteration's bookkeeping is done so
	// the advice is the freshest thing in the next prompt.
	e.consultOracleIfDue(iteration, verifySucceeded)

	// Write a checkpoint last so it captures every state change above.
	e.writeCheckpoint(iteration)

	return nil, nil
}

// snapshotPlan wraps planfile.Take with a no-op fallback when path is empty.
func (e *LoopEngine) snapshotPlan(absPath string) (planfile.Snapshot, error) {
	if absPath == "" {
		return planfile.Snapshot{}, nil
	}
	return planfile.Take(absPath)
}

// runVerify executes the configured verify command (if any) and emits
// a VerifyResultEvent. It returns true when verify succeeded or was
// disabled. A failing verify populates e.lastVerify so the next iteration
// prompt can show the assistant what broke.
func (e *LoopEngine) runVerify(iteration int) bool {
	if e.config.VerifyCmd == "" {
		e.lastVerify = nil
		return true
	}

	res := verify.Run(e.ctx, e.config.VerifyCmd, e.config.WorkingDir, e.config.VerifyTimeout, e.config.VerifyMaxBytes)

	e.emit(NewVerifyResultEvent(
		res.Cmd,
		res.Combined,
		res.ExitCode,
		iteration,
		res.Duration,
		res.Success(),
		res.TimedOut,
	))

	if res.Success() {
		e.lastVerify = nil
		return true
	}

	e.lastVerify = &verifyResultSummary{
		Cmd:      res.Cmd,
		Output:   res.Combined,
		ExitCode: res.ExitCode,
		Success:  false,
		TimedOut: res.TimedOut,
	}
	return false
}

// autoCommit performs the optional `git add -A && git commit` (and tag)
// step. It is a no-op when AutoCommit is false, when the working tree is
// clean, when verify gating is enabled and verify failed, or when git is
// not usable.
func (e *LoopEngine) autoCommit(iteration int, verifySucceeded bool) {
	if !e.config.AutoCommit {
		return
	}
	if e.config.AutoCommitOnVerifyOnly && !verifySucceeded {
		return
	}

	clean, available := e.workingTreeClean()
	if !available || clean {
		return
	}

	msg := e.config.AutoCommitMessage
	if msg == "" {
		msg = "ralph: iteration %d"
	}
	msg = fmt.Sprintf(msg, iteration)

	sha, err := gitutil.CommitAll(e.ctx, e.config.WorkingDir, msg)
	if err != nil || sha == "" {
		return
	}

	tag := ""
	if e.config.AutoTag != "" {
		tag = fmt.Sprintf(e.config.AutoTag, iteration)
		_ = gitutil.CreateTag(e.ctx, e.config.WorkingDir, tag, msg)
	}

	e.emit(NewAutoCommitEvent(sha, msg, tag, iteration))
}

// consultOracleIfDue asks the oracle for advice when the schedule fires
// and stores the result in lastOracleAdvice for the next iteration.
func (e *LoopEngine) consultOracleIfDue(iteration int, verifySucceeded bool) {
	if e.oracle == nil || e.config.OracleModel == "" {
		return
	}
	due := false
	reason := ""
	if e.config.OracleEvery > 0 && iteration%e.config.OracleEvery == 0 {
		due = true
		reason = fmt.Sprintf("scheduled every %d iterations", e.config.OracleEvery)
	}
	if !verifySucceeded && e.config.OracleOnVerifyFail {
		due = true
		reason = "verify failed"
	}
	if !due {
		return
	}

	prompt := e.buildOraclePrompt(iteration, verifySucceeded)
	advice, err := e.oracle.Consult(e.ctx, prompt)
	if err != nil || strings.TrimSpace(advice) == "" {
		return
	}
	e.lastOracleAdvice = strings.TrimSpace(advice)
	e.emit(NewOracleAdviceEvent(e.config.OracleModel, e.lastOracleAdvice, reason, iteration))
}

// buildOraclePrompt assembles the question for the second-opinion model.
func (e *LoopEngine) buildOraclePrompt(iteration int, verifySucceeded bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are a senior engineering reviewer giving a SECOND OPINION on a Ralph loop iteration.\n")
	fmt.Fprintf(&b, "Iteration: %d/%d\n", iteration, e.config.MaxIterations)
	fmt.Fprintf(&b, "Original task:\n%s\n\n", strings.TrimSpace(e.config.Prompt))
	if e.lastSummary != "" {
		fmt.Fprintf(&b, "Last iteration summary:\n%s\n\n", e.lastSummary)
	}
	if !verifySucceeded && e.lastVerify != nil {
		fmt.Fprintf(&b, "Verify failed (exit %d):\n%s\n\n", e.lastVerify.ExitCode, strings.TrimRight(e.lastVerify.Output, "\n"))
	}
	b.WriteString("Reply with concise, actionable advice (max ~10 lines) the next iteration should follow.")
	return b.String()
}

// writeCheckpoint persists current loop state when a checkpoint path is
// configured. Failures are silent so checkpointing never aborts a run.
func (e *LoopEngine) writeCheckpoint(iteration int) {
	if e.config.CheckpointFile == "" {
		return
	}
	state := checkpoint.State{
		Prompt:            e.config.Prompt,
		Model:             e.config.Model,
		WorkingDir:        e.config.WorkingDir,
		PromisePhrase:     e.config.PromisePhrase,
		Iteration:         iteration,
		MaxIterations:     e.config.MaxIterations,
		LastSummary:       e.lastSummary,
		ConsecutiveErrors: e.consecutiveErrors,
		ConsecNoChanges:   e.consecutiveNoChanges,
	}
	if err := checkpoint.Save(e.config.CheckpointFile, state); err != nil {
		return
	}
	e.emit(NewCheckpointSavedEvent(e.config.CheckpointFile, iteration))
}

// workingTreeClean returns whether the working tree is clean. The second
// return value reports whether git was usable at all (false means we should
// skip change-detection rather than treat it as "always dirty" or "always
// clean").
func (e *LoopEngine) workingTreeClean() (clean, available bool) {
	if e.config.WorkingDir == "" {
		return false, false
	}
	clean, err := gitutil.IsClean(e.ctx, e.config.WorkingDir)
	if err != nil {
		return false, false
	}
	return clean, true
}

// updateCarryContext folds the just-finished iteration's response into the
// engine state according to the configured carry-context mode.
func (e *LoopEngine) updateCarryContext(response string) {
	switch e.config.CarryContext {
	case CarryContextOff:
		e.lastSummary = ""
	case CarryContextVerbatim:
		e.lastSummary = truncateSummary(strings.TrimSpace(response), e.config.CarryContextMaxRunes)
	default:
		// CarryContextSummary (and any unknown value) extracts the last
		// <summary> block. When none is present we leave the previous
		// summary alone so the assistant can keep building on it.
		if s := extractSummary(response); s != "" {
			e.lastSummary = truncateSummary(s, e.config.CarryContextMaxRunes)
		}
	}
}

// buildIterationPrompt builds the prompt for the current iteration.
// The system prompt template handles the loop context and completion
// instructions. We layer in the carry-context summary, the running plan
// file, and any specs the user mounted via --specs.
func (e *LoopEngine) buildIterationPrompt(iteration int) string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "[Iteration %d/%d]\n\n", iteration, e.config.MaxIterations)

	if e.config.CarryContext != CarryContextOff && e.lastSummary != "" {
		builder.WriteString("Previous iteration summary:\n")
		builder.WriteString(e.lastSummary)
		builder.WriteString("\n\n")
	}

	if e.lastVerify != nil && !e.lastVerify.Success {
		builder.WriteString("Previous verify command failed; fix the underlying issues before continuing.\n")
		fmt.Fprintf(&builder, "Command: %s\nExit code: %d\n", e.lastVerify.Cmd, e.lastVerify.ExitCode)
		if e.lastVerify.TimedOut {
			builder.WriteString("(verify timed out)\n")
		}
		if strings.TrimSpace(e.lastVerify.Output) != "" {
			builder.WriteString("Output:\n")
			builder.WriteString(strings.TrimRight(e.lastVerify.Output, "\n"))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	if e.config.PlanFile != "" {
		planPath := planfile.Resolve(e.config.WorkingDir, e.config.PlanFile)
		content, err := planfile.Read(planPath)
		if err == nil {
			fmt.Fprintf(&builder, "Running plan (%s):\n", planPath)
			if strings.TrimSpace(content) == "" {
				builder.WriteString("(empty — write your TODO list to this file as you work)\n\n")
			}
			if strings.TrimSpace(content) != "" {
				builder.WriteString(content)
				if !strings.HasSuffix(content, "\n") {
					builder.WriteString("\n")
				}
				builder.WriteString("\n")
			}
		}
	}

	if e.config.SpecsDir != "" {
		list, err := specs.List(e.config.SpecsDir)
		if err == nil && len(list) > 0 {
			fmt.Fprintf(&builder, "Available specs under %s:\n", e.config.SpecsDir)
			for _, s := range list {
				fmt.Fprintf(&builder, "  - %s (%d bytes)\n", s.Rel, s.Bytes)
			}
			builder.WriteString("\n")
		}
	}

	if e.lastOracleAdvice != "" {
		builder.WriteString("Oracle second-opinion (consider before continuing):\n")
		builder.WriteString(strings.TrimSpace(e.lastOracleAdvice))
		builder.WriteString("\n\n")
	}

	for _, p := range e.config.PromptStack {
		if strings.TrimSpace(p) == "" {
			continue
		}
		builder.WriteString(strings.TrimRight(p, "\n"))
		builder.WriteString("\n\n")
	}

	builder.WriteString(e.config.Prompt)

	return builder.String()
}

// complete transitions to the complete state and returns the result.
// This is now only called when max iterations are reached without errors.
func (e *LoopEngine) complete() (*LoopResult, error) {
	e.mu.Lock()
	e.state = StateComplete
	result := e.buildResult()
	result.State = StateComplete
	e.mu.Unlock()

	e.emit(NewLoopCompleteEvent(result))

	return result, nil
}

// fail transitions to the failed state and returns the result with error.
func (e *LoopEngine) fail(err error) (*LoopResult, error) {
	e.mu.Lock()
	e.state = StateFailed
	result := e.buildResult()
	result.State = StateFailed
	result.Error = err
	e.mu.Unlock()

	e.emit(NewLoopFailedEvent(err, result))

	return result, err
}

// cancelled transitions to the cancelled state and returns the result.
func (e *LoopEngine) cancelled() (*LoopResult, error) {
	e.mu.Lock()
	e.state = StateCancelled
	result := e.buildResult()
	result.State = StateCancelled
	result.Error = ErrLoopCancelled
	e.mu.Unlock()

	e.emit(NewLoopCancelledEvent(result))

	return result, ErrLoopCancelled
}

// buildResult creates a LoopResult from current state.
// Must be called with lock held.
func (e *LoopEngine) buildResult() *LoopResult {
	return &LoopResult{
		State:      e.state,
		Iterations: e.iteration,
		Duration:   time.Since(e.startTime),
	}
}

// emit sends an event to the events channel.
func (e *LoopEngine) emit(event any) {
	e.mu.RLock()
	closed := e.eventsClosed
	e.mu.RUnlock()

	if closed {
		return
	}

	select {
	case e.events <- event:
	default:
		// Channel full, event dropped - log warning
	}
}

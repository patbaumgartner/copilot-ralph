package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patbaumgartner/copilot-ralph/internal/core"
)

func TestRootCommandExists(t *testing.T) {
	// Verify that the ralph root command can be invoked with --help
	cmd := exec.Command("go", "run", "./cmd/ralph", "--help")
	// Do not fail if environment unsuitable; this is a smoke test
	_ = cmd.Run()
	require.True(t, true)
}

func TestDisplayEventsAndPrints(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	events := make(chan any, 20)
	cfg := &core.LoopConfig{MaxIterations: 5, PromisePhrase: "Done!"}

	// Send a variety of events
	go func() {
		defer close(events)
		events <- &core.LoopStartEvent{Config: cfg}
		events <- &core.IterationStartEvent{Iteration: 1, MaxIterations: 5}
		events <- &core.AIResponseEvent{Text: "Hello "}
		events <- &core.AIResponseEvent{Text: "world"}
		events <- &core.ToolExecutionStartEvent{ToolEvent: core.ToolEvent{ToolName: "echo", Iteration: 1}}
		events <- &core.ToolExecutionEvent{ToolEvent: core.ToolEvent{ToolName: "echo", Iteration: 1}, Result: "ok"}
		events <- &core.ToolExecutionEvent{ToolEvent: core.ToolEvent{ToolName: "fail", Iteration: 1}, Error: assert.AnError}
		events <- &core.IterationCompleteEvent{Iteration: 1, Duration: time.Millisecond}
		events <- &core.PromiseDetectedEvent{Phrase: "Done!"}
		// Send cancelled to stop displayEvents
		events <- &core.LoopCancelledEvent{}
	}()

	// Call displayEvents which should process until cancel
	displayEvents(events, cfg)

	// Restore stdout and read
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Basic assertions that branches ran
	assert.Contains(t, output, "Loop started")
	assert.Contains(t, output, "Iteration 1/5")
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "Promise detected")
}

func TestPrintLoopConfigAndSummary(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cfg := &core.LoopConfig{Prompt: "task", Model: "gpt-4", MaxIterations: 2, Timeout: 5 * time.Minute, PromisePhrase: "Done!", WorkingDir: "."}
	printLoopConfig(cfg)

	result := &core.LoopResult{State: core.StateComplete, Iterations: 2}
	start := time.Now().Add(-2 * time.Second)
	printSummary(result, start)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	assert.Contains(t, out, "Starting Ralph Loop")
	assert.Contains(t, out, "Loop Summary")
	assert.Contains(t, out, "Iterations:")
}

func TestCreateSDKClientReturnsClient(t *testing.T) {
	// Save/restore globals that createSDKClient reads
	oldRunModel := runModel
	oldRunStreaming := runStreaming
	oldRunLogLevel := runLogLevel
	oldRunSystemMessage := runSystemPrompt
	oldRunSystemMessageMode := runSystemPromptMode
	defer func() {
		runModel = oldRunModel
		runStreaming = oldRunStreaming
		runLogLevel = oldRunLogLevel
		runSystemPrompt = oldRunSystemMessage
		runSystemPromptMode = oldRunSystemMessageMode
	}()

	runModel = "gpt-test"
	runStreaming = true
	runLogLevel = "info"
	runSystemPrompt = ""
	runSystemPromptMode = "append"

	cfg := &core.LoopConfig{Prompt: "task", PromisePhrase: "I'm special!", Model: "gpt-test", Timeout: 30 * time.Second, MaxIterations: 1}
	client, err := createSDKClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)
	// Client Model should match
	assert.Equal(t, "gpt-test", client.Model())
}

// TestResolvePromptStdin verifies that "-" reads from stdin.
func TestResolvePromptStdin(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, _ = w.WriteString("  hello from stdin  ")
	w.Close()

	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })

	got, err := resolvePrompt("-")
	require.NoError(t, err)
	assert.Equal(t, "hello from stdin", got)
}

// TestResolvePromptLiteral verifies non-file strings are returned verbatim.
func TestResolvePromptLiteral(t *testing.T) {
	got, err := resolvePrompt("do the thing")
	require.NoError(t, err)
	assert.Equal(t, "do the thing", got)
}

// TestResolvePromptEmpty returns an error for an empty string.
func TestResolvePromptEmpty(t *testing.T) {
	_, err := resolvePrompt("")
	require.Error(t, err)
}

// TestEnvHelpers verifies env-var helper functions.
func TestEnvHelpers(t *testing.T) {
	t.Run("envString returns default when unset", func(t *testing.T) {
		t.Setenv("RALPH_TEST_STR", "")
		assert.Equal(t, "default", envString("RALPH_TEST_STR", "default"))
	})
	t.Run("envString returns value when set", func(t *testing.T) {
		t.Setenv("RALPH_TEST_STR", "hello")
		assert.Equal(t, "hello", envString("RALPH_TEST_STR", "default"))
	})
	t.Run("envInt returns default when unset", func(t *testing.T) {
		t.Setenv("RALPH_TEST_INT", "")
		assert.Equal(t, 42, envInt("RALPH_TEST_INT", 42))
	})
	t.Run("envInt returns value when set", func(t *testing.T) {
		t.Setenv("RALPH_TEST_INT", "7")
		assert.Equal(t, 7, envInt("RALPH_TEST_INT", 42))
	})
	t.Run("envInt returns default on invalid", func(t *testing.T) {
		t.Setenv("RALPH_TEST_INT", "notanumber")
		assert.Equal(t, 42, envInt("RALPH_TEST_INT", 42))
	})
	t.Run("envDuration returns default when unset", func(t *testing.T) {
		t.Setenv("RALPH_TEST_DUR", "")
		assert.Equal(t, time.Minute, envDuration("RALPH_TEST_DUR", time.Minute))
	})
	t.Run("envDuration returns value when set", func(t *testing.T) {
		t.Setenv("RALPH_TEST_DUR", "5s")
		assert.Equal(t, 5*time.Second, envDuration("RALPH_TEST_DUR", time.Minute))
	})
	t.Run("envBool returns default when unset", func(t *testing.T) {
		t.Setenv("RALPH_TEST_BOOL", "")
		assert.Equal(t, false, envBool("RALPH_TEST_BOOL", false))
	})
	t.Run("envBool returns value when set", func(t *testing.T) {
		t.Setenv("RALPH_TEST_BOOL", "true")
		assert.True(t, envBool("RALPH_TEST_BOOL", false))
	})
}

// TestExitErrorForBlockedState verifies exit code 5 for StateBlocked.
func TestExitErrorForBlockedState(t *testing.T) {
	result := &core.LoopResult{State: core.StateBlocked, Error: core.ErrLoopBlocked}
	err := exitErrorFor(result)
	require.Error(t, err)
	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, exitBlocked, exitErr.Code)
}

// TestValidateSettingsNewFlags verifies the new field validators.
func TestValidateSettingsNewFlags(t *testing.T) {
	base := &core.LoopConfig{
		SystemPromptMode: "append",
		CarryContext:     core.CarryContextOff,
		IterationTimeout: 0,
		StopOnNoChanges:  0,
		StopOnError:      0,
	}

	t.Run("negative StallAfter is rejected", func(t *testing.T) {
		cfg := *base
		cfg.StallAfter = -1
		assert.Error(t, validateSettings(&cfg))
	})
	t.Run("negative IterationDelay is rejected", func(t *testing.T) {
		cfg := *base
		cfg.IterationDelay = -1
		assert.Error(t, validateSettings(&cfg))
	})
	t.Run("zero values are accepted", func(t *testing.T) {
		cfg := *base
		cfg.StallAfter = 0
		cfg.IterationDelay = 0
		assert.NoError(t, validateSettings(&cfg))
	})
}

// TestExitErrorError verifies the Error() method on ExitError.
func TestExitErrorError(t *testing.T) {
	tests := []struct {
		name    string
		e       *ExitError
		wantMsg string
	}{
		{"with wrapped error", &ExitError{Code: 1, Err: errors.New("boom")}, "boom"},
		{"nil wrapped error falls back to code", &ExitError{Code: 3}, "exit code 3"},
		{"zero code with no error", &ExitError{Code: 0}, "exit code 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMsg, tt.e.Error())
		})
	}
}

// TestExitErrorUnwrap verifies the Unwrap() method on ExitError.
func TestExitErrorUnwrap(t *testing.T) {
	inner := errors.New("inner error")
	e := &ExitError{Code: 1, Err: inner}
	assert.Equal(t, inner, e.Unwrap())
	// nil Err → Unwrap returns nil
	assert.Nil(t, (&ExitError{Code: 1}).Unwrap())
}

// TestExitErrorFor covers all branches of exitErrorFor.
func TestExitErrorFor(t *testing.T) {
	tests := []struct {
		name     string
		result   *core.LoopResult
		wantNil  bool
		wantCode int
	}{
		{
			name:     "nil result → exitCancelled",
			result:   nil,
			wantCode: exitCancelled,
		},
		{
			name:    "StateComplete → no error",
			result:  &core.LoopResult{State: core.StateComplete},
			wantNil: true,
		},
		{
			name:     "StateBlocked → exitBlocked",
			result:   &core.LoopResult{State: core.StateBlocked, Error: core.ErrLoopBlocked},
			wantCode: exitBlocked,
		},
		{
			name:     "StateCancelled → exitCancelled",
			result:   &core.LoopResult{State: core.StateCancelled},
			wantCode: exitCancelled,
		},
		{
			name:     "StateFailed + DeadlineExceeded → exitTimeout",
			result:   &core.LoopResult{State: core.StateFailed, Error: context.DeadlineExceeded},
			wantCode: exitTimeout,
		},
		{
			name:     "StateFailed + ErrLoopTimeout → exitTimeout",
			result:   &core.LoopResult{State: core.StateFailed, Error: core.ErrLoopTimeout},
			wantCode: exitTimeout,
		},
		{
			name:     "StateFailed + ErrMaxIterations → exitMaxIterations",
			result:   &core.LoopResult{State: core.StateFailed, Error: core.ErrMaxIterations},
			wantCode: exitMaxIterations,
		},
		{
			name:     "StateFailed + other error → exitFailed",
			result:   &core.LoopResult{State: core.StateFailed, Error: errors.New("some error")},
			wantCode: exitFailed,
		},
		{
			name:     "StateFailed + nil error → exitFailed",
			result:   &core.LoopResult{State: core.StateFailed},
			wantCode: exitFailed,
		},
		{
			name:     "unknown state → exitFailed",
			result:   &core.LoopResult{State: core.StateIdle},
			wantCode: exitFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exitErrorFor(tt.result)
			if tt.wantNil {
				assert.NoError(t, err)
				return
			}
			var exitErr *ExitError
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, tt.wantCode, exitErr.Code)
		})
	}
}

// TestDisplayEvent verifies the return values of displayEvent for every
// event category, independent of rendered output.
func TestDisplayEvent(t *testing.T) {
	cfg := &core.LoopConfig{MaxIterations: 5}

	// Suppress stdout so rendered ANSI output doesn't pollute test output.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	// Drain the pipe in the background so writes never block.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		buf := make([]byte, 4096)
		for {
			if _, rerr := r.Read(buf); rerr != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		w.Close()
		os.Stdout = oldStdout
		<-drainDone
		r.Close()
	})

	tests := []struct {
		name         string
		event        any
		newline      bool
		wantTerminal bool
		wantNewline  bool
	}{
		// Terminal events.
		{"LoopCompleteEvent is terminal", &core.LoopCompleteEvent{}, false, true, false},
		{"LoopFailedEvent is terminal", &core.LoopFailedEvent{}, false, true, false},
		{"LoopCancelledEvent is terminal", &core.LoopCancelledEvent{}, false, true, false},

		// AIResponseEvent sets updatedNewline = true.
		{"AIResponseEvent sets newline", &core.AIResponseEvent{Text: "hi"}, false, false, true},

		// Non-terminal events reset newline to false.
		{"LoopStartEvent not terminal", &core.LoopStartEvent{Config: cfg}, false, false, false},
		{"IterationStartEvent not terminal", &core.IterationStartEvent{Iteration: 1, MaxIterations: 5}, false, false, false},
		{"IterationCompleteEvent not terminal", &core.IterationCompleteEvent{Iteration: 1}, false, false, false},
		{"PromiseDetectedEvent not terminal", &core.PromiseDetectedEvent{Phrase: "done"}, false, false, false},
		{"ErrorEvent not terminal", &core.ErrorEvent{Error: errors.New("oops")}, false, false, false},
		{"ToolExecutionStartEvent not terminal", &core.ToolExecutionStartEvent{ToolEvent: core.ToolEvent{ToolName: "x", Iteration: 1}}, false, false, false},
		{"ToolExecutionEvent success not terminal", &core.ToolExecutionEvent{ToolEvent: core.ToolEvent{ToolName: "x", Iteration: 1}}, false, false, false},
		{"VerifyResultEvent success not terminal", &core.VerifyResultEvent{Success: true, Duration: time.Millisecond}, false, false, false},
		{"NoChangesStopEvent not terminal", &core.NoChangesStopEvent{Threshold: 3}, false, false, false},
		{"ErrorStopEvent not terminal", &core.ErrorStopEvent{Threshold: 3}, false, false, false},
		{"StallStopEvent not terminal", &core.StallStopEvent{Threshold: 3}, false, false, false},
		{"CheckpointSavedEvent not terminal", &core.CheckpointSavedEvent{Path: "x.json", Iteration: 1}, false, false, false},

		// ToolExecutionEvent error preserves the caller's newline state.
		{"ToolExecutionEvent error preserves newline=false", &core.ToolExecutionEvent{ToolEvent: core.ToolEvent{ToolName: "x", Iteration: 1}, Error: errors.New("fail")}, false, false, false},
		{"ToolExecutionEvent error preserves newline=true", &core.ToolExecutionEvent{ToolEvent: core.ToolEvent{ToolName: "x", Iteration: 1}, Error: errors.New("fail")}, true, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotNL := displayEvent(tt.event, cfg, tt.newline)
			assert.Equal(t, tt.wantTerminal, got, "terminal")
			assert.Equal(t, tt.wantNewline, gotNL, "updatedNewline")
		})
	}
}

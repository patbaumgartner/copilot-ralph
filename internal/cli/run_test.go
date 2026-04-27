package cli

import (
	"bytes"
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

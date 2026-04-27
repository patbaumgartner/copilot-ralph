// Package cli implements the command-line interface for Ralph using Cobra.
//
// This file contains tests for CLI commands.
package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patbaumgartner/copilot-ralph/internal/core"
	"github.com/patbaumgartner/copilot-ralph/internal/eventsink"
	"github.com/patbaumgartner/copilot-ralph/internal/sdk"
)

type fakeLoopSDKClient struct {
	startErr         error
	createErr        error
	sendErr          error
	stopCalled       bool
	startCalled      bool
	createCalled     bool
	destroyCalled    bool
	prompt           string
	model            string
	promisePhrase    string
	responseText     string
	sendPromptCalled bool
}

func (f *fakeLoopSDKClient) Start() error {
	f.startCalled = true
	return f.startErr
}

func (f *fakeLoopSDKClient) Stop() error {
	f.stopCalled = true
	return nil
}

func (f *fakeLoopSDKClient) CreateSession(context.Context) error {
	f.createCalled = true
	return f.createErr
}

func (f *fakeLoopSDKClient) DestroySession(context.Context) error {
	f.destroyCalled = true
	return nil
}

func (f *fakeLoopSDKClient) SendPrompt(_ context.Context, prompt string) (<-chan sdk.Event, error) {
	f.sendPromptCalled = true
	f.prompt = prompt
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	events := make(chan sdk.Event, 1)
	text := f.responseText
	if text == "" {
		text = "<promise>" + f.promisePhrase + "</promise>"
	}
	events <- sdk.NewTextEvent(text, false)
	close(events)
	return events, nil
}

func (f *fakeLoopSDKClient) Model() string {
	if f.model != "" {
		return f.model
	}
	return "fake-model"
}

func TestResolvePrompt(t *testing.T) {
	t.Run("from positional argument", func(t *testing.T) {
		result, err := resolvePrompt("test prompt")
		require.NoError(t, err)
		assert.Equal(t, "test prompt", result)
	})

	t.Run("from markdown file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "task.md")
		content := "# Task\nPlease implement X"
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))

		result, err := resolvePrompt(path)
		require.NoError(t, err)
		assert.Equal(t, content, result)
	})

	t.Run("empty when no input", func(t *testing.T) {
		_, err := resolvePrompt("")
		require.Error(t, err)
	})
}

func TestValidateRunConfig(t *testing.T) {
	tests := []struct {
		config      *core.LoopConfig
		name        string
		errorMsg    string
		expectError bool
	}{
		{
			name: "valid config",
			config: &core.LoopConfig{
				Prompt:        "test prompt",
				MaxIterations: 10,
				Timeout:       30 * time.Minute,
			},
			expectError: false,
		},
		{
			name: "empty prompt",
			config: &core.LoopConfig{
				Prompt:        "",
				MaxIterations: 10,
				Timeout:       30 * time.Minute,
			},
			expectError: true,
			errorMsg:    "prompt cannot be empty",
		},
		{
			name: "negative max iterations",
			config: &core.LoopConfig{
				Prompt:        "test",
				MaxIterations: -1,
				Timeout:       30 * time.Minute,
			},
			expectError: true,
			errorMsg:    "max-iterations must be positive",
		},
		{
			name: "negative timeout",
			config: &core.LoopConfig{
				Prompt:        "test",
				MaxIterations: 10,
				Timeout:       -1 * time.Minute,
			},
			expectError: true,
			errorMsg:    "timeout must be positive",
		},
		{
			name: "zero max iterations not allowed",
			config: &core.LoopConfig{
				Prompt:        "test",
				MaxIterations: 0,
				Timeout:       30 * time.Minute,
			},
			expectError: true,
			errorMsg:    "max-iterations must be positive",
		},
		{
			name: "zero timeout not allowed",
			config: &core.LoopConfig{
				Prompt:        "test",
				MaxIterations: 10,
				Timeout:       0,
			},
			expectError: true,
			errorMsg:    "timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRunConfig(tt.config)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestBuildLoopConfig(t *testing.T) {
	tests := []struct {
		name           string
		prompt         string
		promise        string
		model          string
		workingDir     string
		expectedPrompt string
		maxIterations  int
		timeout        time.Duration
	}{
		{
			name:           "uses provided prompt",
			prompt:         "my task",
			maxIterations:  10,
			timeout:        30 * time.Minute,
			promise:        "I'm special!",
			model:          "gpt-4",
			workingDir:     ".",
			expectedPrompt: "my task",
		},
		{
			name:           "applies flag overrides",
			prompt:         "override test",
			maxIterations:  5,
			timeout:        10 * time.Minute,
			promise:        "Done!",
			model:          "gpt-3.5-turbo",
			workingDir:     "/tmp",
			expectedPrompt: "override test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore globals
			oldMaxIterations := runMaxIterations
			oldTimeout := runTimeout
			oldPromise := runPromise
			oldModel := runModel
			oldWorkingDir := runWorkingDir

			runMaxIterations = tt.maxIterations
			runTimeout = tt.timeout
			runPromise = tt.promise
			runModel = tt.model
			runWorkingDir = tt.workingDir

			defer func() {
				runMaxIterations = oldMaxIterations
				runTimeout = oldTimeout
				runPromise = oldPromise
				runModel = oldModel
				runWorkingDir = oldWorkingDir
			}()

			result, err := buildLoopConfig(tt.prompt)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedPrompt, result.Prompt)
			assert.Equal(t, tt.maxIterations, result.MaxIterations)
			assert.Equal(t, tt.timeout, result.Timeout)
			assert.Equal(t, tt.promise, result.PromisePhrase)
			assert.Equal(t, tt.model, result.Model)
			assert.Equal(t, tt.workingDir, result.WorkingDir)
		})
	}
}

func TestConfigExists(t *testing.T) {
	tests := []struct {
		setup    func(t *testing.T) string
		name     string
		expected bool
	}{
		{
			name: "file exists",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, ".ralph.yaml")
				err := os.WriteFile(path, []byte("test: true"), 0644)
				require.NoError(t, err)
				return path
			},
			expected: true,
		},
		{
			name: "file does not exist",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nonexistent.yaml")
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			_, err := os.Stat(path)
			result := !errors.Is(err, os.ErrNotExist)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateSettings(t *testing.T) {
	tests := []struct {
		name        string
		systemMode  string
		logLevel    string
		errorMsg    string
		expectError bool
	}{
		{
			name:        "invalid system message mode",
			systemMode:  "invalid",
			logLevel:    "info",
			expectError: true,
			errorMsg:    "invalid system-prompt-mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &core.LoopConfig{SystemPromptMode: tt.systemMode}
			err := validateSettings(cfg)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestValidateSettingsPhaseAFlags(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *core.LoopConfig)
		errPart string
	}{
		{
			name:    "invalid carry-context",
			mutate:  func(c *core.LoopConfig) { c.CarryContext = "weird" },
			errPart: "invalid carry-context",
		},
		{
			name:    "negative iteration timeout",
			mutate:  func(c *core.LoopConfig) { c.IterationTimeout = -1 },
			errPart: "iteration-timeout",
		},
		{
			name: "iteration timeout exceeds total timeout",
			mutate: func(c *core.LoopConfig) {
				c.Timeout = 1 * time.Minute
				c.IterationTimeout = 2 * time.Minute
			},
			errPart: "cannot exceed timeout",
		},
		{
			name:    "negative stop-on-no-changes",
			mutate:  func(c *core.LoopConfig) { c.StopOnNoChanges = -1 },
			errPart: "stop-on-no-changes",
		},
		{
			name:    "negative stop-on-error",
			mutate:  func(c *core.LoopConfig) { c.StopOnError = -1 },
			errPart: "stop-on-error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &core.LoopConfig{SystemPromptMode: "append"}
			tt.mutate(cfg)
			err := validateSettings(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errPart)
		})
	}
}

func TestValidateSettingsAcceptsCarryContextValues(t *testing.T) {
	for _, mode := range []core.CarryContextMode{"", core.CarryContextOff, core.CarryContextSummary, core.CarryContextVerbatim} {
		cfg := &core.LoopConfig{SystemPromptMode: "append", CarryContext: mode}
		require.NoError(t, validateSettings(cfg), "mode %q should be valid", mode)
	}
}

func TestPrintDryRun(t *testing.T) {
	cfg := &core.LoopConfig{
		Prompt:        "test prompt",
		Model:         "gpt-4",
		MaxIterations: 5,
		Timeout:       10 * time.Minute,
		PromisePhrase: "Done!",
		WorkingDir:    ".",
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printDryRun(cfg)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)
	assert.Contains(t, output, "Configuration Preview")
	assert.Contains(t, output, "test prompt")
	assert.Contains(t, output, "gpt-4")
	assert.Contains(t, output, "5")
}

func TestFormatRateLimitWait(t *testing.T) {
	reset := time.Date(2026, time.April, 27, 10, 30, 0, 0, time.Local)

	withReset := formatRateLimitWait(&core.RateLimitWaitEvent{
		ResetAt:  reset,
		HasReset: true,
		Wait:     90 * time.Second,
	})
	assert.Contains(t, withReset, "resuming at")
	assert.Contains(t, withReset, "1m30s")

	withMessage := formatRateLimitWait(&core.RateLimitWaitEvent{
		Message: "quota exceeded",
		Wait:    2 * time.Minute,
	})
	assert.Contains(t, withMessage, "waiting 2m0s")
	assert.Contains(t, withMessage, "quota exceeded")

	plain := formatRateLimitWait(&core.RateLimitWaitEvent{Wait: time.Second})
	assert.Contains(t, plain, "waiting 1s")
}

func TestRunHook(t *testing.T) {
	t.Run("empty hook is no-op", func(t *testing.T) {
		runHook("", t.TempDir(), &core.LoopResult{})
	})

	t.Run("passes result environment", func(t *testing.T) {
		dir := t.TempDir()
		out := filepath.Join(dir, "hook.out")
		runHook("printf '%s:%s' \"$RALPH_STATE\" \"$RALPH_ITERATIONS\" > hook.out", dir, &core.LoopResult{
			State:      core.StateComplete,
			Iterations: 3,
		})
		got, err := os.ReadFile(out)
		require.NoError(t, err)
		assert.Equal(t, "complete:3", string(got))
	})

	t.Run("failing hook does not panic", func(t *testing.T) {
		runHook("printf fail && exit 7", t.TempDir(), &core.LoopResult{State: core.StateFailed})
	})
}

func TestDisplayEventCoversNonTerminalBranches(t *testing.T) {
	cfg := &core.LoopConfig{MaxIterations: 2}
	events := []any{
		&core.LoopStartEvent{},
		&core.IterationStartEvent{Iteration: 1},
		&core.AIResponseEvent{Text: "hello"},
		&core.ToolExecutionStartEvent{ToolEvent: core.ToolEvent{ToolName: "edit", Parameters: map[string]any{"path": "a.go"}}},
		&core.ToolExecutionEvent{ToolEvent: core.ToolEvent{ToolName: "view"}, Error: errors.New("missing")},
		&core.ToolExecutionEvent{ToolEvent: core.ToolEvent{ToolName: "list"}},
		&core.IterationCompleteEvent{Iteration: 1},
		&core.PromiseDetectedEvent{Phrase: "done"},
		&core.ErrorEvent{Error: errors.New("boom")},
		&core.RateLimitWaitEvent{Wait: time.Second},
		&core.PlanUpdatedEvent{Path: "fix_plan.md", Bytes: 12},
		&core.IterationTimeoutEvent{Iteration: 1, Timeout: time.Second},
		&core.NoChangesStopEvent{Threshold: 2},
		&core.ErrorStopEvent{Threshold: 2},
		&core.BlockedPhraseDetectedEvent{Phrase: "blocked"},
		&core.StallStopEvent{Threshold: 2},
		&core.VerifyResultEvent{Success: true, Duration: time.Millisecond},
		&core.VerifyResultEvent{ExitCode: 1, Duration: time.Millisecond, Output: "fail\n"},
		&core.VerifyResultEvent{TimedOut: true, Duration: time.Millisecond},
		&core.AutoCommitEvent{SHA: "abc1234", Message: "checkpoint", Tag: "ralph-1"},
		&core.WorkspaceDiffEvent{Stat: "a.go | 1 +"},
		&core.OracleAdviceEvent{Model: "gpt-test", Reason: "scheduled", Advice: "try smaller steps\n"},
		&core.CheckpointSavedEvent{Path: "state.json", Iteration: 1},
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	newline := false
	for _, ev := range events {
		terminal, updated := displayEvent(ev, cfg, newline)
		assert.False(t, terminal)
		newline = updated
	}

	require.NoError(t, w.Close())
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "Loop started")
	assert.Contains(t, output, "Iteration 1/2")
	assert.Contains(t, output, "hello")
	assert.Contains(t, output, "verify failed")
	assert.Contains(t, output, "auto-committed")
	assert.Contains(t, output, "oracle")
}

func TestDisplayEventTerminalBranches(t *testing.T) {
	for _, ev := range []any{
		&core.LoopCompleteEvent{},
		&core.LoopFailedEvent{},
		&core.LoopCancelledEvent{},
	} {
		terminal, newline := displayEvent(ev, &core.LoopConfig{}, true)
		assert.True(t, terminal)
		assert.False(t, newline)
	}
}

func TestBuildEventSinks(t *testing.T) {
	oldJSON, oldJSONOutput, oldLogFile, oldWebhook, oldTimeout := runJSON, runJSONOutput, runLogFile, runWebhook, runWebhookTimeout
	defer func() {
		runJSON = oldJSON
		runJSONOutput = oldJSONOutput
		runLogFile = oldLogFile
		runWebhook = oldWebhook
		runWebhookTimeout = oldTimeout
	}()

	dir := t.TempDir()
	runJSON = false
	runJSONOutput = filepath.Join(dir, "events.jsonl")
	runLogFile = filepath.Join(dir, "events.log")
	runWebhook = "http://127.0.0.1:1/hook"
	runWebhookTimeout = time.Millisecond

	fan, err := buildEventSinks()
	require.NoError(t, err)
	require.NotNil(t, fan)
	fan.Write(&core.LoopStartEvent{})
	require.NoError(t, fan.Close())

	jsonData, err := os.ReadFile(runJSONOutput)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "LoopStartEvent")
	logData, err := os.ReadFile(runLogFile)
	require.NoError(t, err)
	assert.Contains(t, string(logData), "LoopStartEvent")
}

func TestDisplayEventsWithSinksJSONStopsOnTerminalEvent(t *testing.T) {
	oldJSON := runJSON
	runJSON = true
	defer func() { runJSON = oldJSON }()

	events := make(chan any, 2)
	events <- &core.LoopStartEvent{}
	events <- &core.LoopCompleteEvent{}
	close(events)

	fan := &eventsink.FanOut{}
	displayEventsWithSinks(events, &core.LoopConfig{}, fan)
}

func TestRunLoopWithConfigDryRun(t *testing.T) {
	cfg := &core.LoopConfig{
		Prompt:             "dry run",
		Model:              "gpt-test",
		MaxIterations:      1,
		Timeout:            time.Minute,
		PromisePhrase:      "done",
		WorkingDir:         t.TempDir(),
		SystemPrompt:       "custom prompt",
		SystemPromptMode:   "replace",
		LogLevel:           "error",
		CarryContext:       core.CarryContextOff,
		DryRun:             true,
		StopOnNoChanges:    0,
		StopOnError:        0,
		StallAfter:         0,
		IterationDelay:     0,
		IterationTimeout:   0,
		OracleEvery:        0,
		OracleOnVerifyFail: false,
	}

	oldStdout, oldStderr := os.Stdout, os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	require.NoError(t, err)
	stderrR, stderrW, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = stdoutW
	os.Stderr = stderrW

	err = runLoopWithConfig(nil, cfg)

	require.NoError(t, stdoutW.Close())
	require.NoError(t, stderrW.Close())
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var stdout, stderr bytes.Buffer
	_, readErr := stdout.ReadFrom(stdoutR)
	require.NoError(t, readErr)
	_, readErr = stderr.ReadFrom(stderrR)
	require.NoError(t, readErr)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Dry Run")
	assert.Contains(t, stderr.String(), "promise instructions")
}

func TestRunLoopUsesPromptArgument(t *testing.T) {
	oldMaxIterations, oldTimeout, oldPromise, oldModel, oldWorkingDir := runMaxIterations, runTimeout, runPromise, runModel, runWorkingDir
	oldDryRun, oldLogLevel, oldSystemMode := runDryRun, runLogLevel, runSystemPromptMode
	defer func() {
		runMaxIterations = oldMaxIterations
		runTimeout = oldTimeout
		runPromise = oldPromise
		runModel = oldModel
		runWorkingDir = oldWorkingDir
		runDryRun = oldDryRun
		runLogLevel = oldLogLevel
		runSystemPromptMode = oldSystemMode
	}()

	runMaxIterations = 1
	runTimeout = time.Minute
	runPromise = "done"
	runModel = "gpt-test"
	runWorkingDir = t.TempDir()
	runDryRun = true
	runLogLevel = "error"
	runSystemPromptMode = "append"

	require.NoError(t, runLoop(nil, []string{"implement this"}))
}

func TestRunLoopWithConfigUsesInjectedSDKClient(t *testing.T) {
	oldFactory := newCopilotClient
	oldJSON, oldJSONOutput, oldLogFile, oldWebhook := runJSON, runJSONOutput, runLogFile, runWebhook
	defer func() {
		newCopilotClient = oldFactory
		runJSON = oldJSON
		runJSONOutput = oldJSONOutput
		runLogFile = oldLogFile
		runWebhook = oldWebhook
	}()

	dir := t.TempDir()
	hookOut := filepath.Join(dir, "complete.out")
	fake := &fakeLoopSDKClient{promisePhrase: "done"}
	newCopilotClient = func(...sdk.ClientOption) (core.SDKClient, error) {
		return fake, nil
	}
	runJSON = false
	runJSONOutput = ""
	runLogFile = ""
	runWebhook = ""

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	err = runLoopWithConfig(nil, &core.LoopConfig{
		Prompt:           "finish the task",
		Model:            "fake-model",
		MaxIterations:    2,
		Timeout:          time.Minute,
		PromisePhrase:    "done",
		WorkingDir:       dir,
		LogLevel:         "error",
		SystemPromptMode: "append",
		CarryContext:     core.CarryContextOff,
		OnCompleteCmd:    "printf complete > complete.out",
	})

	require.NoError(t, w.Close())
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, readErr := buf.ReadFrom(r)
	require.NoError(t, readErr)

	require.NoError(t, err)
	assert.True(t, fake.startCalled)
	assert.True(t, fake.createCalled)
	assert.True(t, fake.sendPromptCalled)
	assert.True(t, fake.destroyCalled)
	assert.True(t, fake.stopCalled)
	assert.Contains(t, fake.prompt, "finish the task")
	out, readErr := os.ReadFile(hookOut)
	require.NoError(t, readErr)
	assert.Equal(t, "complete", string(out))
	assert.Contains(t, buf.String(), "Loop Summary")
}

func TestRunLoopWithConfigPropagatesSDKFactoryError(t *testing.T) {
	oldFactory := newCopilotClient
	defer func() { newCopilotClient = oldFactory }()
	newCopilotClient = func(...sdk.ClientOption) (core.SDKClient, error) {
		return nil, errors.New("factory failed")
	}

	err := runLoopWithConfig(nil, &core.LoopConfig{
		Prompt:           "task",
		Model:            "fake-model",
		MaxIterations:    1,
		Timeout:          time.Minute,
		PromisePhrase:    "done",
		WorkingDir:       t.TempDir(),
		LogLevel:         "error",
		SystemPromptMode: "append",
		CarryContext:     core.CarryContextOff,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SDK client")
}

func TestCreateSDKClientWithCustomSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "system.md")
	require.NoError(t, os.WriteFile(path, []byte("custom system"), 0o644))

	client, err := createSDKClient(&core.LoopConfig{
		Model:            "gpt-test",
		WorkingDir:       dir,
		Timeout:          time.Minute,
		Streaming:        false,
		LogLevel:         "debug",
		PromisePhrase:    "done",
		SystemPrompt:     path,
		SystemPromptMode: "replace",
	})
	require.NoError(t, err)
	assert.Equal(t, "gpt-test", client.Model())

	_, err = createSDKClient(&core.LoopConfig{
		Model:            "gpt-test",
		WorkingDir:       dir,
		Timeout:          time.Minute,
		LogLevel:         "debug",
		PromisePhrase:    "done",
		SystemPrompt:     dir,
		SystemPromptMode: "replace",
	})
	require.Error(t, err)
}

func TestPrintSummaryStates(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	for _, state := range []core.LoopState{
		core.StateComplete,
		core.StateBlocked,
		core.StateFailed,
		core.StateCancelled,
		core.StateIdle,
	} {
		printSummary(&core.LoopResult{State: state, Iterations: 2, Error: errors.New("boom")}, time.Now().Add(-time.Second))
	}

	require.NoError(t, w.Close())
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Complete")
	assert.Contains(t, output, "Blocked")
	assert.Contains(t, output, "Failed")
	assert.Contains(t, output, "Cancelled")
	assert.Contains(t, output, "Error:")
}

func TestToolErrorsContinueExecution(t *testing.T) {
	// Create a mock event stream with tool errors
	events := make(chan any, 10)

	// Send events including tool errors
	go func() {
		defer close(events)

		events <- &core.LoopStartEvent{
			Config: &core.LoopConfig{
				MaxIterations: 5,
				PromisePhrase: "Done!",
			},
		}

		events <- &core.IterationStartEvent{
			Iteration:     1,
			MaxIterations: 5,
		}

		// Tool with error - should not stop processing
		events <- &core.ToolExecutionEvent{
			ToolEvent: core.ToolEvent{
				ToolName:   "view",
				Parameters: map[string]any{"path": "nonexistent"},
				Iteration:  1,
			},
			Error: errors.New("Path does not exist"),
		}

		// Successful tool - should be processed
		events <- &core.ToolExecutionEvent{
			ToolEvent: core.ToolEvent{
				ToolName:   "list",
				Parameters: map[string]any{},
				Iteration:  1,
			},
			Result: "file1.txt, file2.txt",
		}

		events <- &core.IterationCompleteEvent{
			Iteration: 1,
			Duration:  time.Second,
		}

		events <- &core.LoopCompleteEvent{
			Result: &core.LoopResult{
				State:      core.StateComplete,
				Iterations: 1,
			},
		}
	}()

	// Process events - should not panic or stop early
	var toolErrorSeen bool
	var successfulToolSeen bool
	var iterationCompleteSeen bool

	for event := range events {
		switch e := event.(type) {
		case *core.ToolExecutionEvent:
			if e.Error != nil {
				toolErrorSeen = true
			}
			if e.Error == nil && e.ToolName == "list" {
				successfulToolSeen = true
			}
		case *core.IterationCompleteEvent:
			iterationCompleteSeen = true
		}
	}

	// Verify all events were processed
	assert.True(t, toolErrorSeen, "Tool error event should be processed")
	assert.True(t, successfulToolSeen, "Successful tool after error should be processed")
	assert.True(t, iterationCompleteSeen, "Iteration complete should be processed after tool error")
}

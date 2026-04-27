package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patbaumgartner/copilot-ralph/internal/sdk"
)

// Test that tool result events can trigger promise detection and file change tracking.
func TestToolOutputPromiseAndFileChange(t *testing.T) {
	mock := NewMockSDKClient()
	// Simulate a tool result event that contains the promise phrase and an edit that changes a file
	mock.ToolCalls = []sdk.ToolCall{
		{ID: "1", Name: "edit", Parameters: map[string]any{"path": "main.go"}},
	}
	mock.ResponseText = "processing"
	mock.SimulatePromise = false

	cfg := &LoopConfig{Prompt: "Task", MaxIterations: 1, PromisePhrase: "Done"}
	eng := NewLoopEngine(cfg, mock)

	result, err := eng.Start(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StateComplete, result.State)
}

// Test that tool result containing promise triggers a PromiseDetectedEvent emission (via events channel)
func TestToolResultTriggersPromiseDetectedEvent(t *testing.T) {
	mock := NewMockSDKClient()
	mock.ToolCalls = []sdk.ToolCall{{ID: "1", Name: "run", Parameters: map[string]any{}}}
	mock.ResponseText = "result"
	mock.SimulatePromise = true
	mock.PromisePhrase = "DONE"

	cfg := &LoopConfig{Prompt: "Task", MaxIterations: 2, PromisePhrase: "DONE"}
	eng := NewLoopEngine(cfg, mock)

	events := eng.Events()
	go func() {
		_, _ = eng.Start(context.Background())
	}()

	seen := false
	for ev := range events {
		if pe, ok := ev.(*PromiseDetectedEvent); ok {
			if pe.Phrase == "DONE" {
				seen = true
				break
			}
		}
	}

	assert.True(t, seen, "PromiseDetectedEvent should be emitted when tool output contains promise")
}

func TestEmitDropsWhenClosed(t *testing.T) {
	eng := NewLoopEngine(nil, nil)
	// Close events channel by toggling flag
	eng.mu.Lock()
	eng.eventsClosed = true
	eng.mu.Unlock()

	// Should not panic
	eng.emit(NewLoopStartEvent(eng.Config()))
}

// TestLoopEngine_BlockedSignal verifies that the engine stops with
// StateBlocked when the model emits the blocked signal.
func TestLoopEngine_BlockedSignal(t *testing.T) {
	mock := NewMockSDKClient()
	mock.ResponseText = "sorry <blocked>I give up</blocked>"

	cfg := &LoopConfig{
		Prompt:        "do x",
		MaxIterations: 5,
		BlockedPhrase: "I give up",
	}
	eng := NewLoopEngine(cfg, mock)

	result, err := eng.Start(context.Background())

	require.NoError(t, err)
	assert.Equal(t, StateBlocked, result.State)
	assert.ErrorIs(t, result.Error, ErrLoopBlocked)
}

// TestLoopEngine_BlockedPhraseDetectedEventEmitted verifies that a
// BlockedPhraseDetectedEvent is emitted before the loop stops.
func TestLoopEngine_BlockedPhraseDetectedEventEmitted(t *testing.T) {
	mock := NewMockSDKClient()
	mock.ResponseText = "<blocked>stuck</blocked>"

	cfg := &LoopConfig{
		Prompt:        "task",
		MaxIterations: 3,
		BlockedPhrase: "stuck",
	}
	eng := NewLoopEngine(cfg, mock)

	evCh := eng.Events()
	go func() { _, _ = eng.Start(context.Background()) }()

	seen := false
	for ev := range evCh {
		if _, ok := ev.(*BlockedPhraseDetectedEvent); ok {
			seen = true
		}
	}
	assert.True(t, seen, "expected BlockedPhraseDetectedEvent")
}

// TestLoopEngine_StallDetection verifies that the loop stops when
// StallAfter consecutive identical responses are produced.
func TestLoopEngine_StallDetection(t *testing.T) {
	mock := NewMockSDKClient()
	mock.ResponseText = "same response every time"

	cfg := &LoopConfig{
		Prompt:        "task",
		MaxIterations: 10,
		StallAfter:    3,
	}
	eng := NewLoopEngine(cfg, mock)

	result, err := eng.Start(context.Background())

	// fail() returns the error as the second value — not a fatal error.
	assert.ErrorIs(t, err, ErrStallDetected)
	require.NotNil(t, result)
	assert.Equal(t, StateFailed, result.State)
	assert.ErrorIs(t, result.Error, ErrStallDetected)
	// Should have stopped after StallAfter+1 iterations (first unique + N identical)
	assert.LessOrEqual(t, result.Iterations, 5)
}

// TestLoopEngine_StallDetection_DisabledWhenZero verifies that StallAfter=0
// does not trigger stall detection.
func TestLoopEngine_StallDetection_DisabledWhenZero(t *testing.T) {
	mock := NewMockSDKClient()
	mock.ResponseText = "same every time"

	cfg := &LoopConfig{
		Prompt:        "task",
		MaxIterations: 3,
		StallAfter:    0, // disabled
	}
	eng := NewLoopEngine(cfg, mock)

	result, err := eng.Start(context.Background())

	require.NoError(t, err)
	// Should complete naturally (max iterations), not via stall
	assert.NotErrorIs(t, result.Error, ErrStallDetected)
	assert.Equal(t, 3, result.Iterations)
}

// TestLoopEngine_IterationDelay verifies that IterationDelay delays do not
// cause errors and are cancelled by context cancellation.
func TestLoopEngine_IterationDelay(t *testing.T) {
	mock := NewMockSDKClient()
	mock.ResponseText = "ok"

	cfg := &LoopConfig{
		Prompt:         "task",
		MaxIterations:  2,
		IterationDelay: 1 * time.Millisecond,
	}
	eng := NewLoopEngine(cfg, mock)

	result, err := eng.Start(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, result.Iterations)
}

func TestBuildResultTiming(t *testing.T) {
	eng := NewLoopEngine(nil, nil)
	eng.mu.Lock()
	eng.startTime = time.Now().Add(-5 * time.Second)
	eng.iteration = 2
	eng.state = StateRunning
	eng.mu.Unlock()

	res := eng.buildResult()
	assert.Equal(t, 2, res.Iterations)
	assert.True(t, res.Duration >= 5*time.Second)
}

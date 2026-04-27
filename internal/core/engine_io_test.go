package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patbaumgartner/copilot-ralph/internal/checkpoint"
)

type stubOracle struct {
	called  int
	advice  string
	lastReq string
	err     error
}

func (s *stubOracle) Consult(_ context.Context, prompt string) (string, error) {
	s.called++
	s.lastReq = prompt
	return s.advice, s.err
}

func TestNewLoopEngineSeedsResumeState(t *testing.T) {
	cfg := &LoopConfig{
		Prompt:              "task",
		MaxIterations:       5,
		ResumeFromIteration: 3,
		ResumeSummary:       "carry context",
	}
	eng := NewLoopEngine(cfg, nil)

	assert.Equal(t, 3, eng.iteration)
	assert.Equal(t, "carry context", eng.lastSummary)
}

func TestSetOracleAttachesClient(t *testing.T) {
	eng := NewLoopEngine(&LoopConfig{}, nil)
	o := &stubOracle{advice: "x"}
	eng.SetOracle(o)
	require.NotNil(t, eng.oracle)
}

func TestConsultOracleScheduled(t *testing.T) {
	o := &stubOracle{advice: "be careful"}
	cfg := &LoopConfig{
		Prompt:        "do x",
		MaxIterations: 10,
		OracleModel:   "gpt-x",
		OracleEvery:   2,
	}
	eng := NewLoopEngine(cfg, nil)
	eng.SetOracle(o)
	eng.ctx = context.Background()

	// Iteration 1 not due, iteration 2 due.
	eng.consultOracleIfDue(1, true)
	assert.Equal(t, 0, o.called)

	eng.consultOracleIfDue(2, true)
	assert.Equal(t, 1, o.called)
	assert.Equal(t, "be careful", eng.lastOracleAdvice)
}

func TestConsultOracleOnVerifyFail(t *testing.T) {
	o := &stubOracle{advice: "retry"}
	cfg := &LoopConfig{
		OracleModel:        "gpt-x",
		OracleEvery:        0, // no schedule
		OracleOnVerifyFail: true,
	}
	eng := NewLoopEngine(cfg, nil)
	eng.SetOracle(o)
	eng.ctx = context.Background()

	eng.consultOracleIfDue(1, true) // not due, verify ok
	assert.Equal(t, 0, o.called)

	eng.consultOracleIfDue(1, false) // due, verify failed
	assert.Equal(t, 1, o.called)
}

func TestConsultOracleEmptyAdviceNoEvent(t *testing.T) {
	o := &stubOracle{advice: "  "}
	cfg := &LoopConfig{OracleModel: "m", OracleEvery: 1}
	eng := NewLoopEngine(cfg, nil)
	eng.SetOracle(o)
	eng.ctx = context.Background()

	eng.consultOracleIfDue(1, true)
	assert.Equal(t, 1, o.called)
	assert.Equal(t, "", eng.lastOracleAdvice)
}

func TestConsultOracleErrorIsSilent(t *testing.T) {
	o := &stubOracle{err: errors.New("boom")}
	cfg := &LoopConfig{OracleModel: "m", OracleEvery: 1}
	eng := NewLoopEngine(cfg, nil)
	eng.SetOracle(o)
	eng.ctx = context.Background()

	eng.consultOracleIfDue(1, true)
	assert.Equal(t, "", eng.lastOracleAdvice)
}

func TestWriteCheckpointEmitsEventAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ck.json")
	cfg := &LoopConfig{
		Prompt:         "build x",
		Model:          "gpt-y",
		WorkingDir:     dir,
		MaxIterations:  4,
		CheckpointFile: path,
	}
	eng := NewLoopEngine(cfg, nil)
	eng.lastSummary = "did 1"

	eng.writeCheckpoint(2)

	state, err := checkpoint.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "build x", state.Prompt)
	assert.Equal(t, 2, state.Iteration)
	assert.Equal(t, "did 1", state.LastSummary)

	// Drain buffered events and find CheckpointSavedEvent.
	saw := false
	for {
		select {
		case ev := <-eng.Events():
			if ck, ok := ev.(*CheckpointSavedEvent); ok && ck.Path == path && ck.Iteration == 2 {
				saw = true
			}
			continue
		default:
		}
		break
	}
	assert.True(t, saw, "expected CheckpointSavedEvent to be emitted")
}

func TestWriteCheckpointNoOpWithoutPath(t *testing.T) {
	cfg := &LoopConfig{Prompt: "x"}
	eng := NewLoopEngine(cfg, nil)
	eng.writeCheckpoint(1) // must not panic
}

func TestBuildIterationPromptIncludesOracleAdviceAndStack(t *testing.T) {
	cfg := &LoopConfig{
		Prompt:        "primary task",
		MaxIterations: 1,
		PromptStack:   []string{"  follow style guide  ", "no TODOs"},
	}
	eng := NewLoopEngine(cfg, nil)
	eng.lastOracleAdvice = "consider edge cases"

	got := eng.buildIterationPrompt(1)

	assert.Contains(t, got, "Oracle second-opinion")
	assert.Contains(t, got, "consider edge cases")
	assert.Contains(t, got, "follow style guide")
	assert.Contains(t, got, "no TODOs")
	assert.Contains(t, got, "primary task")
}

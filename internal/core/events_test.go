package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestToolEventInfo(t *testing.T) {
	// No parameters
	e := &ToolEvent{ToolName: "echo", Parameters: map[string]any{}, Iteration: 1}
	info := e.Info("!")
	assert.Equal(t, "! echo", info)

	// With parameters - values should be present in the returned info
	params := map[string]any{"path": "file.txt", "line": 42}
	e2 := &ToolEvent{ToolName: "edit", Parameters: params, Iteration: 2}
	info2 := e2.Info("🔧")
	assert.Contains(t, info2, "edit")
	assert.Contains(t, info2, "file.txt")
	assert.Contains(t, info2, "42")
}

func TestNewRateLimitWaitEvent(t *testing.T) {
	resetAt := timeMustParse("2026-04-27T01:07:00Z")
	ev := NewRateLimitWaitEvent("hit limit", "rate_limit", resetAt, true, 60_000_000_000, 3)
	assert.Equal(t, "hit limit", ev.Message)
	assert.Equal(t, "rate_limit", ev.ErrorType)
	assert.Equal(t, resetAt, ev.ResetAt)
	assert.True(t, ev.HasReset)
	assert.Equal(t, 3, ev.Iteration)
}

func TestSimpleEventConstructors(t *testing.T) {
	plan := NewPlanUpdatedEvent("/tmp/fix_plan.md", 42, 2)
	assert.Equal(t, "/tmp/fix_plan.md", plan.Path)
	assert.Equal(t, 42, plan.Bytes)
	assert.Equal(t, 2, plan.Iteration)

	noChanges := NewNoChangesStopEvent(3, 7)
	assert.Equal(t, 3, noChanges.Threshold)
	assert.Equal(t, 7, noChanges.Iteration)

	timeout := NewIterationTimeoutEvent(5*time.Second, 4)
	assert.Equal(t, 5*time.Second, timeout.Timeout)
	assert.Equal(t, 4, timeout.Iteration)

	diff := NewWorkspaceDiffEvent("1 file changed", 6)
	assert.Equal(t, "1 file changed", diff.Stat)
	assert.Equal(t, 6, diff.Iteration)
}

func timeMustParse(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

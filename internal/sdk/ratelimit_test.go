package sdk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsRateLimitErrorType(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"rate_limit", "rate_limit", true},
		{"rate-limit dash", "rate-limit", true},
		{"ratelimit", "ratelimit", true},
		{"quota", "quota", true},
		{"quota_exceeded", "quota_exceeded", true},
		{"mixed case", "Rate_Limit", true},
		{"with whitespace", "  quota  ", true},
		{"authentication", "authentication", false},
		{"unknown", "context_limit", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRateLimitErrorType(tt.in))
		})
	}
}

func TestIsRateLimitMessage(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"rate limit phrase", "You've used 93% of your session rate limit", true},
		{"rate_limit token", "error: rate_limit hit", true},
		{"quota", "Monthly quota exceeded", true},
		{"too many requests", "429 too many requests", true},
		{"unrelated", "connection reset by peer", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRateLimitMessage(tt.in))
		})
	}
}

func TestParseRateLimitReset(t *testing.T) {
	now := time.Date(2026, time.April, 27, 0, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		msg      string
		wantTime time.Time
		wantOk   bool
	}{
		{
			name:     "year-less april with am",
			msg:      "Your session rate limit will reset on April 27 at 1:07 AM. Learn More",
			wantOk:   true,
			wantTime: time.Date(2026, time.April, 27, 1, 7, 0, 0, time.UTC),
		},
		{
			name:     "year-less abbreviated month",
			msg:      "rate limit will reset on Apr 27 at 1:07 AM",
			wantOk:   true,
			wantTime: time.Date(2026, time.April, 27, 1, 7, 0, 0, time.UTC),
		},
		{
			name:     "rfc3339 explicit",
			msg:      "rate limit will reset on 2026-04-27T01:07:00Z",
			wantOk:   true,
			wantTime: time.Date(2026, time.April, 27, 1, 7, 0, 0, time.UTC),
		},
		{
			name:   "no reset clause",
			msg:    "rate limit reached",
			wantOk: false,
		},
		{
			name:   "garbled",
			msg:    "resets at sometime soon",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRateLimitReset(tt.msg, now)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.True(t, got.Equal(tt.wantTime), "got %s want %s", got, tt.wantTime)
			}
		})
	}
}

func TestResolveRateLimitWait(t *testing.T) {
	now := time.Date(2026, time.April, 27, 0, 0, 0, 0, time.UTC)

	t.Run("no reset uses fallback", func(t *testing.T) {
		got := resolveRateLimitWait(time.Time{}, false, now)
		assert.Equal(t, rateLimitFallbackWait, got)
	})

	t.Run("future reset adds buffer", func(t *testing.T) {
		reset := now.Add(2 * time.Minute)
		got := resolveRateLimitWait(reset, true, now)
		assert.Equal(t, 2*time.Minute+rateLimitBuffer, got)
	})

	t.Run("past reset uses buffer only", func(t *testing.T) {
		reset := now.Add(-time.Minute)
		got := resolveRateLimitWait(reset, true, now)
		assert.Equal(t, rateLimitBuffer, got)
	})

	t.Run("very far reset is clamped", func(t *testing.T) {
		reset := now.Add(24 * time.Hour)
		got := resolveRateLimitWait(reset, true, now)
		assert.Equal(t, rateLimitMaxWait, got)
	})
}

func TestSleepCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok := sleepCtx(ctx, 10*time.Second)
	assert.False(t, ok)
}

func TestSleepCtxCompletes(t *testing.T) {
	ok := sleepCtx(context.Background(), 5*time.Millisecond)
	assert.True(t, ok)
}

func TestRateLimitEvent(t *testing.T) {
	resetAt := time.Date(2026, time.April, 27, 1, 7, 0, 0, time.UTC)
	ev := NewRateLimitEvent("hit limit", "rate_limit", resetAt, true, time.Minute)
	require.NotNil(t, ev)
	assert.Equal(t, EventTypeRateLimit, ev.Type())
	assert.Equal(t, "hit limit", ev.Message)
	assert.Equal(t, "rate_limit", ev.ErrorType)
	assert.True(t, ev.HasReset)
	assert.Equal(t, resetAt, ev.ResetAt)
	assert.Equal(t, time.Minute, ev.Wait)
	assert.WithinDuration(t, time.Now(), ev.Timestamp(), time.Second)
}

func TestConsumeRateLimitFallsBackToErrorMessage(t *testing.T) {
	c := &CopilotClient{}
	err := errors.New("SDK error: You've used 100% of your session rate limit. Reset on April 27 at 1:07 AM")
	info, ok := c.consumeRateLimit(err)
	require.True(t, ok)
	assert.Equal(t, err.Error(), info.message)
	assert.True(t, info.hasReset)
}

func TestConsumeRateLimitNonRateLimitError(t *testing.T) {
	c := &CopilotClient{}
	_, ok := c.consumeRateLimit(errors.New("connection reset"))
	assert.False(t, ok)
}

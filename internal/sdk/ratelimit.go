// Package sdk provides rate-limit detection and parsing helpers.
package sdk

import (
	"regexp"
	"strings"
	"time"
)

// rateLimitErrorTypes is the set of SessionErrorData.ErrorType values that
// indicate the assistant cannot proceed until a quota or rate-limit window
// resets.
var rateLimitErrorTypes = map[string]struct{}{
	"rate_limit":     {},
	"rate-limit":     {},
	"ratelimit":      {},
	"quota":          {},
	"quota_exceeded": {},
	"quota-exceeded": {},
}

// isRateLimitErrorType reports whether the SDK-reported error category is a
// rate-limit / quota error.
func isRateLimitErrorType(errType string) bool {
	if errType == "" {
		return false
	}
	_, ok := rateLimitErrorTypes[strings.ToLower(strings.TrimSpace(errType))]
	return ok
}

// isRateLimitMessage reports whether a free-form error or message string
// describes a Copilot rate-limit / quota condition.
func isRateLimitMessage(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "rate limit") {
		return true
	}
	if strings.Contains(lower, "rate_limit") {
		return true
	}
	if strings.Contains(lower, "ratelimit") {
		return true
	}
	if strings.Contains(lower, "quota") {
		return true
	}
	if strings.Contains(lower, "too many requests") {
		return true
	}
	return false
}

// Default and bounding waits used when the upstream message does not include
// a usable reset timestamp.
const (
	rateLimitFallbackWait = 5 * time.Minute
	rateLimitMaxWait      = 65 * time.Minute
	rateLimitBuffer       = 30 * time.Second
)

// resetPhrasePattern captures the date/time portion of messages like
//
//	"Your session rate limit will reset on April 27 at 1:07 AM"
//	"will reset on 2026-04-27T01:07:00Z"
//	"resets at 2026-04-27 01:07"
var resetPhrasePattern = regexp.MustCompile(`(?i)reset(?:s| on| at)?\s+(?:on\s+)?([^.\n]+)`)

// resetDateLayouts lists the time.Parse layouts we attempt when reading a
// reset timestamp out of a free-form message.
var resetDateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"January 2 at 3:04 PM",
	"January 2 at 15:04",
	"Jan 2 at 3:04 PM",
	"Jan 2 at 15:04",
	"January 2, 2006 at 3:04 PM",
	"January 2 2006 at 3:04 PM",
	"January 2, 2006 3:04 PM",
}

// parseRateLimitReset attempts to parse the reset timestamp from a Copilot
// rate-limit / quota message. It returns the reset time and true when a
// timestamp can be recovered, otherwise the zero time and false.
//
// now is used to anchor relative ("today") or year-less ("April 27") values
// in the local timezone.
func parseRateLimitReset(msg string, now time.Time) (time.Time, bool) {
	if msg == "" {
		return time.Time{}, false
	}

	match := resetPhrasePattern.FindStringSubmatch(msg)
	if len(match) < 2 {
		return time.Time{}, false
	}

	candidate := strings.TrimSpace(match[1])
	candidate = strings.TrimSuffix(candidate, ".")
	candidate = strings.TrimSpace(candidate)

	// Strip trailing "Learn More" / "(UTC)" decorations.
	for _, suffix := range []string{"Learn More", "Learn more", "(UTC)", "UTC"} {
		candidate = strings.TrimSpace(strings.TrimSuffix(candidate, suffix))
	}

	loc := now.Location()

	for _, layout := range resetDateLayouts {
		t, err := time.ParseInLocation(layout, candidate, loc)
		if err != nil {
			continue
		}
		// Year-less layouts default to year 0; pin them to the current
		// (or next) calendar year.
		if t.Year() == 0 {
			t = time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
			if t.Before(now.Add(-12 * time.Hour)) {
				t = t.AddDate(1, 0, 0)
			}
		}
		return t, true
	}

	return time.Time{}, false
}

// resolveRateLimitWait determines how long to sleep before retrying. It
// prefers the explicit reset timestamp (clamped to a safety bound), falling
// back to a default duration when no timestamp is available. A small buffer
// is added so we resume just after the limit resets.
func resolveRateLimitWait(resetAt time.Time, hasReset bool, now time.Time) time.Duration {
	if !hasReset {
		return rateLimitFallbackWait
	}

	wait := resetAt.Sub(now) + rateLimitBuffer
	if wait <= 0 {
		return rateLimitBuffer
	}
	if wait > rateLimitMaxWait {
		return rateLimitMaxWait
	}
	return wait
}

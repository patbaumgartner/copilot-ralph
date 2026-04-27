// Package core provides summary extraction for iteration carry-context.
package core

import (
	"regexp"
	"strings"
)

// summaryPattern matches a `<summary>...</summary>` block. The (?s) flag
// allows the inner content to span multiple lines.
var summaryPattern = regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)

// extractSummary returns the inner text of the LAST `<summary>...</summary>`
// block in the given response, trimmed of leading/trailing whitespace. It
// returns an empty string when no block is present.
//
// The last block wins so that if the assistant emits multiple summaries
// (for example one per major change) the most recent one is carried forward.
func extractSummary(text string) string {
	matches := summaryPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return ""
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return ""
	}
	return strings.TrimSpace(last[1])
}

// truncateSummary clamps a summary to maxRunes runes (not bytes) so that an
// overly chatty assistant cannot blow up the next iteration's prompt budget.
// A trailing ellipsis is appended when truncation occurs. maxRunes <= 0
// disables truncation.
func truncateSummary(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

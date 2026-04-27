// Package core provides promise detection for the loop engine.
package core

import (
	"fmt"
	"strings"
)

// detectPromise checks whether text contains the wrapped promise marker.
//
// The loop's system prompt instructs the assistant to emit the promise inside
// `<promise>...</promise>` tags. detectPromise performs an exact case- and
// punctuation-sensitive substring match on that wrapped form to avoid false
// positives from echoes of the promise phrase elsewhere in the conversation.
func detectPromise(text, promisePhrase string) bool {
	if promisePhrase == "" {
		return false
	}

	promisePhrase = fmt.Sprintf("<promise>%s</promise>", promisePhrase)

	return strings.Contains(text, promisePhrase)
}

// detectBlocked checks whether text contains the wrapped blocked marker.
//
// When the assistant cannot proceed it emits `<blocked>...</blocked>` with
// the configured blocked phrase inside. This uses the same exact substring
// match as detectPromise to avoid false positives.
func detectBlocked(text, blockedPhrase string) bool {
	if blockedPhrase == "" {
		return false
	}

	wrapped := fmt.Sprintf("<blocked>%s</blocked>", blockedPhrase)

	return strings.Contains(text, wrapped)
}

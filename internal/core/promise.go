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

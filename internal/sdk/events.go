// Package sdk provides event types for Copilot SDK communication.
package sdk

import (
	"time"
)

// ToolCall represents a tool invocation request from the assistant.
type ToolCall struct {
	Parameters map[string]any
	ID         string
	Name       string
}

// EventType represents the type of event from the Copilot SDK.
type EventType string

const (
	// EventTypeText indicates a text/streaming content event.
	EventTypeText EventType = "text"
	// EventTypeToolCall indicates a tool invocation request event.
	EventTypeToolCall EventType = "tool_call"
	// EventTypeToolResult indicates a tool execution result event.
	EventTypeToolResult EventType = "tool_result"
	// EventTypeResponseComplete indicates the response is complete.
	EventTypeResponseComplete EventType = "response_complete"
	// EventTypeError indicates an error occurred.
	EventTypeError EventType = "error"
	// EventTypeRateLimit indicates the session hit a rate or quota limit
	// and the client is waiting before retrying.
	EventTypeRateLimit EventType = "rate_limit"
)

// Event represents an event from the Copilot SDK.
// All event types implement this interface.
type Event interface {
	// Type returns the event type.
	Type() EventType
	// Timestamp returns when the event occurred.
	Timestamp() time.Time
}

// TextEvent represents a text/streaming content event.
type TextEvent struct {
	timestamp time.Time
	Text      string
	Reasoning bool
}

// Type returns EventTypeText.
func (e *TextEvent) Type() EventType {
	return EventTypeText
}

// Timestamp returns when the event occurred.
func (e *TextEvent) Timestamp() time.Time {
	return e.timestamp
}

// NewTextEvent creates a new TextEvent with the given text.
func NewTextEvent(text string, reasoning bool) *TextEvent {
	return &TextEvent{
		Text:      text,
		Reasoning: reasoning,
		timestamp: time.Now(),
	}
}

// ToolCallEvent represents a tool invocation request from the assistant.
type ToolCallEvent struct {
	// ToolCall contains the tool call details.
	ToolCall  ToolCall
	timestamp time.Time
}

// Type returns EventTypeToolCall.
func (e *ToolCallEvent) Type() EventType {
	return EventTypeToolCall
}

// Timestamp returns when the event occurred.
func (e *ToolCallEvent) Timestamp() time.Time {
	return e.timestamp
}

// NewToolCallEvent creates a new ToolCallEvent with the given tool call.
func NewToolCallEvent(toolCall ToolCall) *ToolCallEvent {
	return &ToolCallEvent{
		ToolCall:  toolCall,
		timestamp: time.Now(),
	}
}

// ToolResultEvent represents the result of a tool execution.
type ToolResultEvent struct {
	ToolCall  ToolCall
	timestamp time.Time
	Error     error
	Result    string
}

// Type returns EventTypeToolResult.
func (e *ToolResultEvent) Type() EventType {
	return EventTypeToolResult
}

// Timestamp returns when the event occurred.
func (e *ToolResultEvent) Timestamp() time.Time {
	return e.timestamp
}

// NewToolResultEvent creates a new ToolResultEvent with the given result.
func NewToolResultEvent(toolCall ToolCall, result string, err error) *ToolResultEvent {
	return &ToolResultEvent{
		ToolCall:  toolCall,
		Result:    result,
		Error:     err,
		timestamp: time.Now(),
	}
}

// ErrorEvent represents an error that occurred during processing.
type ErrorEvent struct {
	// Err contains the error that occurred.
	Err       error
	timestamp time.Time
}

// Type returns EventTypeError.
func (e *ErrorEvent) Type() EventType {
	return EventTypeError
}

// Timestamp returns when the event occurred.
func (e *ErrorEvent) Timestamp() time.Time {
	return e.timestamp
}

// Error returns the error message.
func (e *ErrorEvent) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// NewErrorEvent creates a new ErrorEvent with the given error.
func NewErrorEvent(err error) *ErrorEvent {
	return &ErrorEvent{
		Err:       err,
		timestamp: time.Now(),
	}
}

// RateLimitEvent indicates the session hit a Copilot rate limit or quota
// boundary. The SDK client emits it before sleeping until ResetAt and
// retrying the prompt.
type RateLimitEvent struct {
	// ResetAt is when the rate limit is expected to reset. If HasReset is
	// false, callers should fall back to Wait.
	ResetAt time.Time
	// Wait is the duration the SDK will sleep before retrying.
	Wait time.Duration
	// Message is the original message reported by the SDK or upstream.
	Message string
	// ErrorType is the SDK-reported category (e.g. "rate_limit", "quota").
	// Empty when detected from a free-form error string.
	ErrorType string
	timestamp time.Time
	// HasReset indicates whether ResetAt is meaningful.
	HasReset bool
}

// Type returns EventTypeRateLimit.
func (e *RateLimitEvent) Type() EventType {
	return EventTypeRateLimit
}

// Timestamp returns when the event occurred.
func (e *RateLimitEvent) Timestamp() time.Time {
	return e.timestamp
}

// NewRateLimitEvent creates a RateLimitEvent.
func NewRateLimitEvent(message, errorType string, resetAt time.Time, hasReset bool, wait time.Duration) *RateLimitEvent {
	return &RateLimitEvent{
		Message:   message,
		ErrorType: errorType,
		ResetAt:   resetAt,
		HasReset:  hasReset,
		Wait:      wait,
		timestamp: time.Now(),
	}
}

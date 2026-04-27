// Package eventsink fans out core loop events to optional auxiliary
// destinations: a JSON Lines file/stream, a webhook URL, and/or a plain
// text log file. Sinks never block the engine — write errors are
// recorded on the sink and surfaced via Errors() so the caller can show
// them once the loop has finished.
package eventsink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"
)

// Sink is anything that can record a loop event. Implementations must be
// safe for concurrent use and must never block the caller longer than
// strictly necessary.
type Sink interface {
	// Write records a single event. Returning an error is informational;
	// the calling fan-out will log it but not stop processing.
	Write(event any) error
	// Close releases any resources owned by the sink.
	Close() error
}

// FanOut multiplexes events to several sinks. The zero value is ready
// to use; add sinks with Add and fan an event in via Write.
type FanOut struct {
	mu     sync.Mutex
	sinks  []Sink
	errors []error
}

// Add registers a sink. Nil sinks are ignored so callers can pass
// New*Sink results directly without nil-checking.
func (f *FanOut) Add(s Sink) {
	if s == nil {
		return
	}
	f.mu.Lock()
	f.sinks = append(f.sinks, s)
	f.mu.Unlock()
}

// Write sends the event to every sink, collecting any errors.
func (f *FanOut) Write(event any) {
	f.mu.Lock()
	sinks := append([]Sink(nil), f.sinks...)
	f.mu.Unlock()

	for _, s := range sinks {
		if err := s.Write(event); err != nil {
			f.mu.Lock()
			f.errors = append(f.errors, err)
			f.mu.Unlock()
		}
	}
}

// Errors returns a snapshot of all sink errors observed so far.
func (f *FanOut) Errors() []error {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]error, len(f.errors))
	copy(out, f.errors)
	return out
}

// Close closes every sink and returns the first error encountered.
func (f *FanOut) Close() error {
	f.mu.Lock()
	sinks := f.sinks
	f.sinks = nil
	f.mu.Unlock()

	var firstErr error
	for _, s := range sinks {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Envelope is the JSON shape used for both file and webhook output. The
// "type" field is the unqualified Go type name (e.g. "IterationStartEvent")
// to keep downstream consumers stable across refactors of the core
// package's import path.
type Envelope struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Event     any       `json:"event"`
}

// envelopeFor wraps an event in the canonical JSON shape.
func envelopeFor(event any) Envelope {
	t := reflect.TypeOf(event)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := ""
	if t != nil {
		name = t.Name()
	}
	return Envelope{
		Type:      name,
		Timestamp: time.Now().UTC(),
		Event:     event,
	}
}

// JSONSink writes one Envelope per line to the underlying writer. It is
// safe for concurrent use and flushes on every write so a crash never
// loses more than the in-flight event.
type JSONSink struct {
	mu     sync.Mutex
	w      io.Writer
	closer io.Closer
}

// NewJSONSink wraps an existing writer (e.g. os.Stdout) as a sink. The
// writer is not closed on Close.
func NewJSONSink(w io.Writer) *JSONSink {
	return &JSONSink{w: w}
}

// NewJSONFileSink opens path for writing (truncating) and returns a sink
// that closes the file on Close.
func NewJSONFileSink(path string) (*JSONSink, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("open json sink %s: %w", path, err)
	}
	return &JSONSink{w: f, closer: f}, nil
}

// Write encodes one envelope on its own line.
func (s *JSONSink) Write(event any) error {
	if s == nil || s.w == nil {
		return nil
	}
	data, err := json.Marshal(envelopeFor(event))
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}

// Close closes the underlying file (if any).
func (s *JSONSink) Close() error {
	if s == nil || s.closer == nil {
		return nil
	}
	return s.closer.Close()
}

// LogFileSink mirrors any string-formattable event to a plain text file.
// It records each event as "<timestamp> <Type>" plus a per-type one-line
// summary, suitable for grepping. Detailed payloads belong in JSON.
type LogFileSink struct {
	mu sync.Mutex
	f  *os.File
}

// NewLogFileSink opens path for append (creating if missing).
func NewLogFileSink(path string) (*LogFileSink, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log sink %s: %w", path, err)
	}
	return &LogFileSink{f: f}, nil
}

// Write writes a one-line summary of the event.
func (s *LogFileSink) Write(event any) error {
	if s == nil || s.f == nil {
		return nil
	}
	t := reflect.TypeOf(event)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := ""
	if t != nil {
		name = t.Name()
	}
	line := fmt.Sprintf("%s %s %+v\n", time.Now().UTC().Format(time.RFC3339Nano), name, event)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.f.WriteString(line); err != nil {
		return fmt.Errorf("write log: %w", err)
	}
	return nil
}

// Close closes the underlying file.
func (s *LogFileSink) Close() error {
	if s == nil || s.f == nil {
		return nil
	}
	return s.f.Close()
}

// WebhookSink POSTs each envelope as JSON to the configured URL. It uses
// a single in-flight goroutine guarded by mu so events are delivered in
// order without blocking the caller's goroutine for longer than the HTTP
// round-trip.
type WebhookSink struct {
	mu      sync.Mutex
	url     string
	client  *http.Client
	timeout time.Duration
}

// NewWebhookSink configures a sink that POSTs envelopes to url. timeout
// caps a single delivery; <=0 selects 5 seconds.
func NewWebhookSink(url string, timeout time.Duration) *WebhookSink {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookSink{
		url:     url,
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// Write delivers one envelope. Errors are returned but do not stop the
// loop; the FanOut records them.
func (s *WebhookSink) Write(event any) error {
	if s == nil || s.url == "" {
		return nil
	}
	data, err := json.Marshal(envelopeFor(event))
	if err != nil {
		return fmt.Errorf("marshal webhook event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ralph/eventsink")

	s.mu.Lock()
	defer s.mu.Unlock()

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}

// Close is a no-op; the http.Client is reused safely.
func (s *WebhookSink) Close() error { return nil }

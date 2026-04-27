package eventsink

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type sampleEvent struct {
	Iteration int
	Note      string
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type errCloser struct{}

func (errCloser) Close() error { return errors.New("close failed") }

type errSink struct {
	writeErr error
	closeErr error
}

func (s errSink) Write(any) error { return s.writeErr }
func (s errSink) Close() error    { return s.closeErr }

func TestJSONSinkWritesOneLinePerEvent(t *testing.T) {
	var buf bytes.Buffer
	s := NewJSONSink(&buf)
	if err := s.Write(&sampleEvent{Iteration: 1, Note: "hi"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := s.Write(&sampleEvent{Iteration: 2, Note: "ho"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), buf.String())
	}
	var env Envelope
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != "sampleEvent" {
		t.Fatalf("expected sampleEvent type, got %q", env.Type)
	}
}

func TestJSONFileSink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	s, err := NewJSONFileSink(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.Write(&sampleEvent{Iteration: 7}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), `"Iteration":7`) {
		t.Fatalf("expected payload, got %q", data)
	}
}

func TestLogFileSinkAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	s, err := NewLogFileSink(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.Write(&sampleEvent{Iteration: 1}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "sampleEvent") {
		t.Fatalf("expected event name in log, got %q", data)
	}
}

func TestWebhookSinkPostsJSON(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		var env Envelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		if env.Type != "sampleEvent" {
			t.Errorf("type: %q", env.Type)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewWebhookSink(srv.URL, time.Second)
	if err := s.Write(&sampleEvent{Iteration: 9}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", hits)
	}
}

func TestWebhookSinkErrorOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := NewWebhookSink(srv.URL, time.Second)
	if err := s.Write(&sampleEvent{}); err == nil {
		t.Fatalf("expected error on 5xx")
	}
}

func TestFanOutAggregatesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fan := &FanOut{}
	fan.Add(NewJSONSink(io.Discard))
	fan.Add(NewWebhookSink(srv.URL, time.Second))

	fan.Write(&sampleEvent{})

	if errs := fan.Errors(); len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if err := fan.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestJSONSinkNoopsAndErrorPaths(t *testing.T) {
	if err := (*JSONSink)(nil).Write(&sampleEvent{}); err != nil {
		t.Fatalf("nil sink write: %v", err)
	}
	if err := NewJSONSink(nil).Write(&sampleEvent{}); err != nil {
		t.Fatalf("nil writer write: %v", err)
	}
	if err := (*JSONSink)(nil).Close(); err != nil {
		t.Fatalf("nil sink close: %v", err)
	}

	if err := NewJSONSink(io.Discard).Write(make(chan int)); err == nil {
		t.Fatalf("expected marshal error")
	}
	if err := NewJSONSink(errWriter{}).Write(&sampleEvent{}); err == nil {
		t.Fatalf("expected write error")
	}

	s := &JSONSink{w: io.Discard, closer: errCloser{}}
	if err := s.Close(); err == nil {
		t.Fatalf("expected close error")
	}
}

func TestNewFileSinkErrors(t *testing.T) {
	dir := t.TempDir()
	parentFile := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}

	if _, err := NewJSONFileSink(filepath.Join(parentFile, "events.jsonl")); err == nil {
		t.Fatalf("expected json file sink error")
	}
	if _, err := NewLogFileSink(filepath.Join(parentFile, "events.log")); err == nil {
		t.Fatalf("expected log file sink error")
	}
}

func TestLogFileSinkNoopsAndErrorPaths(t *testing.T) {
	if err := (*LogFileSink)(nil).Write(&sampleEvent{}); err != nil {
		t.Fatalf("nil sink write: %v", err)
	}
	if err := (&LogFileSink{}).Write(&sampleEvent{}); err != nil {
		t.Fatalf("nil file write: %v", err)
	}
	if err := (*LogFileSink)(nil).Close(); err != nil {
		t.Fatalf("nil sink close: %v", err)
	}
	if err := (&LogFileSink{}).Close(); err != nil {
		t.Fatalf("nil file close: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "closed.log")
	s, err := NewLogFileSink(path)
	if err != nil {
		t.Fatalf("new log sink: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := s.Write(&sampleEvent{}); err == nil {
		t.Fatalf("expected write error after close")
	}
}

func TestWebhookSinkNoopsAndRequestErrors(t *testing.T) {
	if err := (*WebhookSink)(nil).Write(&sampleEvent{}); err != nil {
		t.Fatalf("nil sink write: %v", err)
	}
	if err := NewWebhookSink("", 0).Write(&sampleEvent{}); err != nil {
		t.Fatalf("empty url write: %v", err)
	}

	if err := NewWebhookSink("http://example.invalid", time.Second).Write(make(chan int)); err == nil {
		t.Fatalf("expected marshal error")
	}
	if err := NewWebhookSink(":// bad-url", time.Second).Write(&sampleEvent{}); err == nil {
		t.Fatalf("expected request build error")
	}
}

func TestFanOutNilSinkAndCloseError(t *testing.T) {
	fan := &FanOut{}
	fan.Add(nil)
	fan.Add(errSink{writeErr: errors.New("write failed"), closeErr: errors.New("close failed")})

	fan.Write(&sampleEvent{})
	if errs := fan.Errors(); len(errs) != 1 {
		t.Fatalf("expected one write error, got %d", len(errs))
	}
	if err := fan.Close(); err == nil {
		t.Fatalf("expected close error")
	}
	if err := fan.Close(); err != nil {
		t.Fatalf("second close should be nil, got %v", err)
	}
}

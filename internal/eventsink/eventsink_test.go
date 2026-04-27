package eventsink

import (
	"bytes"
	"encoding/json"
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

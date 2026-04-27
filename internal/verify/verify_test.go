package verify

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	r := Run(context.Background(), "echo hello", t.TempDir(), 0, 0)
	if !r.Success() {
		t.Fatalf("expected success, got %+v", r)
	}
	if !strings.Contains(r.Stdout, "hello") {
		t.Fatalf("stdout missing: %q", r.Stdout)
	}
	if !strings.Contains(r.Combined, "hello") {
		t.Fatalf("combined missing: %q", r.Combined)
	}
}

func TestRunFailure(t *testing.T) {
	r := Run(context.Background(), "echo boom 1>&2; exit 7", t.TempDir(), 0, 0)
	if r.Success() {
		t.Fatalf("expected failure, got success")
	}
	if r.ExitCode != 7 {
		t.Fatalf("expected exit 7, got %d", r.ExitCode)
	}
	if !strings.Contains(r.Stderr, "boom") {
		t.Fatalf("stderr missing: %q", r.Stderr)
	}
}

func TestRunTimeout(t *testing.T) {
	r := Run(context.Background(), "sleep 5", t.TempDir(), 100*time.Millisecond, 0)
	if !r.TimedOut {
		t.Fatalf("expected timeout, got %+v", r)
	}
	if r.Err == nil {
		t.Fatalf("expected error on timeout")
	}
}

func TestRunEmptyCmd(t *testing.T) {
	r := Run(context.Background(), "  ", t.TempDir(), 0, 0)
	if r.Err == nil {
		t.Fatalf("expected error for empty command")
	}
}

func TestCappedWriter(t *testing.T) {
	r := Run(context.Background(), "printf 'aaaaaaaaaa'", t.TempDir(), 0, 4)
	if !strings.HasPrefix(r.Stdout, "aaaa") {
		t.Fatalf("expected prefix retained, got %q", r.Stdout)
	}
	if !strings.Contains(r.Stdout, "[truncated]") {
		t.Fatalf("expected truncation marker, got %q", r.Stdout)
	}
}

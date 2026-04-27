package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/patbaumgartner/copilot-ralph/internal/cli"
)

func TestRunSuccess(t *testing.T) {
	var buf bytes.Buffer
	oldExecute, oldStderr := executeCLI, stderr
	executeCLI = func() error { return nil }
	stderr = &buf
	t.Cleanup(func() {
		executeCLI = oldExecute
		stderr = oldStderr
	})

	if code := run(); code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", buf.String())
	}
}

func TestRunExitErrorWithCause(t *testing.T) {
	var buf bytes.Buffer
	oldExecute, oldStderr := executeCLI, stderr
	executeCLI = func() error {
		return &cli.ExitError{Code: 5, Err: errors.New("blocked")}
	}
	stderr = &buf
	t.Cleanup(func() {
		executeCLI = oldExecute
		stderr = oldStderr
	})

	if code := run(); code != 5 {
		t.Fatalf("run() = %d, want 5", code)
	}
	if got := buf.String(); got != "Error: blocked\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRunExitErrorWithoutCause(t *testing.T) {
	var buf bytes.Buffer
	oldExecute, oldStderr := executeCLI, stderr
	executeCLI = func() error { return &cli.ExitError{Code: 2} }
	stderr = &buf
	t.Cleanup(func() {
		executeCLI = oldExecute
		stderr = oldStderr
	})

	if code := run(); code != 2 {
		t.Fatalf("run() = %d, want 2", code)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", buf.String())
	}
}

func TestRunGenericError(t *testing.T) {
	var buf bytes.Buffer
	oldExecute, oldStderr := executeCLI, stderr
	executeCLI = func() error { return errors.New("boom") }
	stderr = &buf
	t.Cleanup(func() {
		executeCLI = oldExecute
		stderr = oldStderr
	})

	if code := run(); code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	if got := buf.String(); got != "Error: boom\n" {
		t.Fatalf("stderr = %q", got)
	}
}

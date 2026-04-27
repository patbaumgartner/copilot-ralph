// Package verify runs a user-supplied shell command after each loop
// iteration and reports its outcome. The result is folded into the next
// iteration's prompt so the assistant can react to test/build failures.
package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Result captures the outcome of a verify run.
type Result struct {
	Cmd      string
	Stdout   string
	Stderr   string
	Combined string
	ExitCode int
	Duration time.Duration
	TimedOut bool
	Err      error
}

// Success reports whether the verify command exited cleanly.
func (r *Result) Success() bool {
	return r != nil && r.ExitCode == 0 && r.Err == nil && !r.TimedOut
}

// Run executes cmd via `sh -c` from workingDir with the given timeout
// (<=0 means no timeout). It captures stdout, stderr, and a merged stream
// (truncated to maxBytes per stream when maxBytes > 0).
func Run(ctx context.Context, cmd, workingDir string, timeout time.Duration, maxBytes int) *Result {
	if strings.TrimSpace(cmd) == "" {
		return &Result{Cmd: cmd, Err: errors.New("empty verify command")}
	}

	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	start := time.Now()
	c := exec.CommandContext(runCtx, "sh", "-c", cmd)
	c.Dir = workingDir

	var stdout, stderr, combined bytes.Buffer
	c.Stdout = newCappedWriter(&stdout, &combined, maxBytes)
	c.Stderr = newCappedWriter(&stderr, &combined, maxBytes)

	err := c.Run()
	dur := time.Since(start)

	res := &Result{
		Cmd:      cmd,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Combined: combined.String(),
		Duration: dur,
		Err:      err,
	}

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.Err = fmt.Errorf("verify command timed out after %s", timeout)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		}
		return res
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		res.Err = nil
		return res
	}
	if err == nil {
		res.ExitCode = 0
	}
	return res
}

// cappedWriter writes to a primary buffer (per-stream) and a shared
// combined buffer until each cap is hit. After the cap, writes are
// silently dropped. Cap <= 0 means unlimited.
type cappedWriter struct {
	primary  *bytes.Buffer
	combined *bytes.Buffer
	maxBytes int
}

func newCappedWriter(primary, combined *bytes.Buffer, maxBytes int) *cappedWriter {
	return &cappedWriter{primary: primary, combined: combined, maxBytes: maxBytes}
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	w.writeBounded(w.primary, p)
	w.writeBounded(w.combined, p)
	return len(p), nil
}

func (w *cappedWriter) writeBounded(buf *bytes.Buffer, p []byte) {
	if w.maxBytes <= 0 {
		buf.Write(p)
		return
	}
	remaining := w.maxBytes - buf.Len()
	if remaining <= 0 {
		return
	}
	if remaining >= len(p) {
		buf.Write(p)
		return
	}
	buf.Write(p[:remaining])
	buf.WriteString("\n…[truncated]\n")
}

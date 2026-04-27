package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func initRepoForEngine(t *testing.T) string {
	t.Helper()
	skipIfNoGit(t)
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "ralph@example.com"},
		{"config", "user.name", "Ralph"},
		{"commit", "--allow-empty", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestRunVerifySuccessClearsLastVerify(t *testing.T) {
	cfg := DefaultLoopConfig()
	cfg.WorkingDir = t.TempDir()
	cfg.VerifyCmd = "true"
	e := NewLoopEngine(cfg, nil)
	e.ctx = context.Background()
	e.lastVerify = &verifyResultSummary{Success: false, Cmd: "old"}

	go func() {
		for range e.events { //nolint:revive
		}
	}()

	if !e.runVerify(1) {
		t.Fatalf("expected verify success")
	}
	if e.lastVerify != nil {
		t.Fatalf("expected lastVerify cleared")
	}
}

func TestRunVerifyFailurePopulatesLastVerify(t *testing.T) {
	cfg := DefaultLoopConfig()
	cfg.WorkingDir = t.TempDir()
	cfg.VerifyCmd = "echo nope; exit 3"
	e := NewLoopEngine(cfg, nil)
	e.ctx = context.Background()

	go func() {
		for range e.events { //nolint:revive
		}
	}()

	if e.runVerify(1) {
		t.Fatalf("expected verify failure")
	}
	if e.lastVerify == nil || e.lastVerify.ExitCode != 3 {
		t.Fatalf("expected lastVerify populated with exit 3, got %+v", e.lastVerify)
	}

	prompt := e.buildIterationPrompt(2)
	if !strings.Contains(prompt, "Previous verify command failed") {
		t.Fatalf("expected verify failure folded into prompt: %q", prompt)
	}
	if !strings.Contains(prompt, "Exit code: 3") {
		t.Fatalf("expected exit code in prompt: %q", prompt)
	}
}

func TestAutoCommitOnVerifySuccess(t *testing.T) {
	dir := initRepoForEngine(t)

	cfg := DefaultLoopConfig()
	cfg.WorkingDir = dir
	cfg.AutoCommit = true
	cfg.AutoCommitOnVerifyOnly = true
	cfg.AutoCommitMessage = "ralph: iter %d"

	e := NewLoopEngine(cfg, nil)
	e.ctx = context.Background()

	// Make working tree dirty.
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := make(chan any, 4)
	go func() {
		for ev := range e.events {
			got <- ev
		}
		close(got)
	}()

	e.autoCommit(7, true)

	// Drain a single event with timeout.
	select {
	case ev := <-got:
		ac, ok := ev.(*AutoCommitEvent)
		if !ok {
			t.Fatalf("expected AutoCommitEvent, got %T", ev)
		}
		if !strings.Contains(ac.Message, "iter 7") {
			t.Fatalf("expected formatted message, got %q", ac.Message)
		}
		if ac.SHA == "" {
			t.Fatalf("expected SHA")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected AutoCommitEvent")
	}
}

func TestAutoCommitSkipsOnFailedVerifyByDefault(t *testing.T) {
	dir := initRepoForEngine(t)
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := DefaultLoopConfig()
	cfg.WorkingDir = dir
	cfg.AutoCommit = true
	cfg.AutoCommitOnVerifyOnly = true
	e := NewLoopEngine(cfg, nil)
	e.ctx = context.Background()

	emitted := false
	done := make(chan struct{})
	go func() {
		for ev := range e.events {
			if _, ok := ev.(*AutoCommitEvent); ok {
				emitted = true
			}
		}
		close(done)
	}()

	e.autoCommit(1, false)

	// Close the events channel by exercising emit's no-op then explicitly
	// shutting down to let the goroutine exit. We close manually here.
	e.mu.Lock()
	e.eventsClosed = true
	e.mu.Unlock()
	close(e.events)
	<-done

	if emitted {
		t.Fatalf("did not expect AutoCommitEvent when verify failed")
	}
}

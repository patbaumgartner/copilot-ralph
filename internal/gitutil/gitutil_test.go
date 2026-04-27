package gitutil

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	skipIfNoGit(t)
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "ralph@example.com"},
		{"config", "user.name", "Ralph"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestIsRepoFalseOutsideRepo(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	ok, err := IsRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("IsRepo: %v", err)
	}
	if ok {
		t.Fatalf("expected false outside repo")
	}
}

func TestIsCleanFreshRepo(t *testing.T) {
	dir := initRepo(t)
	clean, err := IsClean(context.Background(), dir)
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	if !clean {
		t.Fatalf("expected fresh repo to be clean")
	}
}

func TestIsCleanDetectsDirty(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	clean, err := IsClean(context.Background(), dir)
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	if clean {
		t.Fatalf("expected dirty working tree")
	}
}

func TestPorcelainStatusErrOutsideRepo(t *testing.T) {
	skipIfNoGit(t)
	_, err := PorcelainStatus(context.Background(), t.TempDir())
	if !errors.Is(err, ErrNotARepo) {
		t.Fatalf("expected ErrNotARepo, got %v", err)
	}
}

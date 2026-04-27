package gitutil

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// initRepoWithCommit creates a repo with an initial commit so HEAD exists.
func initRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-q", "-m", "init"},
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

func TestDiffStatCleanRepo(t *testing.T) {
	dir := initRepoWithCommit(t)
	stat, err := DiffStat(context.Background(), dir)
	if err != nil {
		t.Fatalf("DiffStat: %v", err)
	}
	if stat != "" {
		t.Fatalf("expected empty diff stat on clean repo, got %q", stat)
	}
}

func TestDiffStatFreshRepoNoHead(t *testing.T) {
	// Brand-new repo has no HEAD commit; DiffStat should return ("", nil).
	dir := initRepo(t)
	stat, err := DiffStat(context.Background(), dir)
	if err != nil {
		t.Fatalf("DiffStat on no-HEAD repo: %v", err)
	}
	if stat != "" {
		t.Fatalf("expected empty diff stat when no HEAD, got %q", stat)
	}
}

func TestDiffStatNonRepo(t *testing.T) {
	skipIfNoGit(t)
	_, err := DiffStat(context.Background(), t.TempDir())
	if !errors.Is(err, ErrNotARepo) {
		t.Fatalf("expected ErrNotARepo, got %v", err)
	}
}

func TestCommitAllCleanRepo(t *testing.T) {
	dir := initRepoWithCommit(t)
	sha, err := CommitAll(context.Background(), dir, "should not commit")
	if err != nil {
		t.Fatalf("CommitAll on clean repo: %v", err)
	}
	if sha != "" {
		t.Fatalf("expected empty sha on clean repo, got %q", sha)
	}
}

func TestCommitAllDirtyRepo(t *testing.T) {
	dir := initRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("change"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	sha, err := CommitAll(context.Background(), dir, "ralph: iteration 1")
	if err != nil {
		t.Fatalf("CommitAll: %v", err)
	}
	if sha == "" {
		t.Fatal("expected non-empty SHA after commit")
	}
	if len(sha) < 4 {
		t.Fatalf("SHA too short: %q", sha)
	}
}

func TestCommitAllNonRepo(t *testing.T) {
	skipIfNoGit(t)
	_, err := CommitAll(context.Background(), t.TempDir(), "msg")
	if !errors.Is(err, ErrNotARepo) {
		t.Fatalf("expected ErrNotARepo, got %v", err)
	}
}

func TestCreateTagCreatesTag(t *testing.T) {
	dir := initRepoWithCommit(t)
	if err := CreateTag(context.Background(), dir, "v1.0.0", "first release"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	// Verify tag exists
	cmd := exec.Command("git", "-C", dir, "tag", "-l", "v1.0.0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git tag -l: %v", err)
	}
	if !strings.Contains(string(out), "v1.0.0") {
		t.Fatal("tag v1.0.0 not found after CreateTag")
	}
}

func TestCreateTagIdempotent(t *testing.T) {
	dir := initRepoWithCommit(t)
	if err := CreateTag(context.Background(), dir, "v1.0.0", "first"); err != nil {
		t.Fatalf("first CreateTag: %v", err)
	}
	// Second call must be a no-op, not an error.
	if err := CreateTag(context.Background(), dir, "v1.0.0", "second"); err != nil {
		t.Fatalf("second CreateTag (idempotent): %v", err)
	}
}

func TestCreateTagNonRepo(t *testing.T) {
	skipIfNoGit(t)
	err := CreateTag(context.Background(), t.TempDir(), "v1.0.0", "msg")
	// ensureGit passes; CreateTag calls rev-parse which exits non-zero →
	// falls through to `git tag` which fails with a non-nil error.
	if err == nil {
		t.Fatal("expected error tagging non-repo")
	}
}

func TestGitNotFoundErrors(t *testing.T) {
	t.Setenv("PATH", "")
	ctx := context.Background()
	dir := t.TempDir()

	if err := ensureGit(); !errors.Is(err, ErrGitNotFound) {
		t.Fatalf("ensureGit = %v, want ErrGitNotFound", err)
	}
	if _, err := IsRepo(ctx, dir); !errors.Is(err, ErrGitNotFound) {
		t.Fatalf("IsRepo = %v, want ErrGitNotFound", err)
	}
	if _, err := PorcelainStatus(ctx, dir); !errors.Is(err, ErrGitNotFound) {
		t.Fatalf("PorcelainStatus = %v, want ErrGitNotFound", err)
	}
	if _, err := DiffStat(ctx, dir); !errors.Is(err, ErrGitNotFound) {
		t.Fatalf("DiffStat = %v, want ErrGitNotFound", err)
	}
	if _, err := CommitAll(ctx, dir, "msg"); !errors.Is(err, ErrGitNotFound) {
		t.Fatalf("CommitAll = %v, want ErrGitNotFound", err)
	}
	if err := CreateTag(ctx, dir, "v1", "msg"); !errors.Is(err, ErrGitNotFound) {
		t.Fatalf("CreateTag = %v, want ErrGitNotFound", err)
	}
}

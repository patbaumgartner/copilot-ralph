// Package gitutil provides minimal git helpers used by the Ralph loop:
// repo detection, working-tree dirtiness checks, and short status output.
//
// Everything is implemented by shelling out to the `git` binary so we do not
// pick up a heavy dependency. All commands are run from a caller-supplied
// working directory.
package gitutil

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNotARepo indicates the working directory is not inside a git repository.
var ErrNotARepo = errors.New("not a git repository")

// ErrGitNotFound indicates the `git` binary could not be located on PATH.
var ErrGitNotFound = errors.New("git binary not found on PATH")

// ensureGit returns ErrGitNotFound when git is missing from PATH.
func ensureGit() error {
	if _, err := exec.LookPath("git"); err != nil {
		return ErrGitNotFound
	}
	return nil
}

// IsRepo reports whether dir is inside a git repository. It returns
// ErrGitNotFound when git is unavailable.
func IsRepo(ctx context.Context, dir string) (bool, error) {
	if err := ensureGit(); err != nil {
		return false, err
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// git prints to stderr and exits non-zero outside a repo; treat that
		// as "not a repo" rather than a hard error.
		return false, nil //nolint:nilerr
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// PorcelainStatus returns the output of `git status --porcelain` for dir.
// An empty string means the working tree is clean. ErrNotARepo is returned
// when dir is not inside a git repository.
func PorcelainStatus(ctx context.Context, dir string) (string, error) {
	if err := ensureGit(); err != nil {
		return "", err
	}
	ok, err := IsRepo(ctx, dir)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotARepo
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// IsClean reports whether the working tree at dir has no uncommitted changes.
func IsClean(ctx context.Context, dir string) (bool, error) {
	status, err := PorcelainStatus(ctx, dir)
	if err != nil {
		return false, fmt.Errorf("is clean: %w", err)
	}
	return status == "", nil
}

// DiffStat returns the output of `git diff --stat HEAD` for dir, capturing
// both staged and unstaged changes. An empty string means no diff. The
// "Bytes" field on Result holds the byte count of the captured diff.
func DiffStat(ctx context.Context, dir string) (string, error) {
	if err := ensureGit(); err != nil {
		return "", err
	}
	ok, err := IsRepo(ctx, dir)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotARepo
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--stat", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		// In a brand-new repo HEAD does not yet exist; surface an empty
		// diff rather than an error in that case.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", nil
		}
		return "", fmt.Errorf("git diff: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// CommitAll stages every change in dir and creates a commit with the given
// message. It returns the resulting commit's short SHA. ErrNotARepo is
// returned when dir is not inside a git repository. A clean working tree
// returns ("", nil) — there is nothing to commit.
func CommitAll(ctx context.Context, dir, message string) (string, error) {
	if err := ensureGit(); err != nil {
		return "", err
	}
	ok, err := IsRepo(ctx, dir)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotARepo
	}

	clean, err := IsClean(ctx, dir)
	if err != nil {
		return "", err
	}
	if clean {
		return "", nil
	}

	addCmd := exec.CommandContext(ctx, "git", "-C", dir, "add", "-A")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	commitCmd := exec.CommandContext(ctx, "git", "-C", dir, "commit", "-m", message)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	shaCmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--short", "HEAD")
	out, err := shaCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CreateTag creates an annotated tag pointing at HEAD. It is a no-op when
// the tag already exists.
func CreateTag(ctx context.Context, dir, tag, message string) error {
	if err := ensureGit(); err != nil {
		return err
	}
	checkCmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "-q", "--verify", "refs/tags/"+tag)
	if err := checkCmd.Run(); err == nil {
		return nil // tag already exists
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "tag", "-a", tag, "-m", message)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git tag: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

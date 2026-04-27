package planfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	workDir := t.TempDir()
	absElsewhere := filepath.Join(t.TempDir(), "elsewhere", "plan.md")
	tests := []struct {
		name    string
		workDir string
		path    string
		want    string
	}{
		{"empty uses default", workDir, "", filepath.Join(workDir, ".ralph", "fix_plan.md")},
		{"absolute kept", workDir, absElsewhere, absElsewhere},
		{"relative joined", workDir, "plan.md", filepath.Join(workDir, "plan.md")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.workDir, tt.path)
			if got != tt.want {
				t.Fatalf("Resolve(%q, %q) = %q, want %q", tt.workDir, tt.path, got, tt.want)
			}
		})
	}
}

func TestReadMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got, err := Read(filepath.Join(dir, "nope.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty content, got %q", got)
	}
}

func TestEnsureDirAndTake(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "sub", ".ralph", "fix_plan.md")

	if err := EnsureDir(plan); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	pre, err := Take(plan)
	if err != nil {
		t.Fatalf("Take pre: %v", err)
	}
	if pre.Exists {
		t.Fatalf("expected plan not to exist yet")
	}

	if err := os.WriteFile(plan, []byte("- todo one\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	post, err := Take(plan)
	if err != nil {
		t.Fatalf("Take post: %v", err)
	}
	if !post.Exists {
		t.Fatalf("expected plan to exist")
	}
	if post.Content != "- todo one\n" {
		t.Fatalf("unexpected content: %q", post.Content)
	}

	if !Changed(pre, post) {
		t.Fatalf("expected Changed to be true")
	}

	if Changed(post, post) {
		t.Fatalf("expected Changed to be false for identical snapshots")
	}
}

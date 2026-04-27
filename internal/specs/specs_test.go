package specs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListEmptyDirReturnsNil(t *testing.T) {
	got, err := List("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for empty dir, got %v", got)
	}
}

func TestListMissingDirError(t *testing.T) {
	_, err := List(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatalf("expected error for missing dir")
	}
}

func TestListNotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.md")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := List(file)
	if err == nil {
		t.Fatalf("expected error when path is a file")
	}
}

func TestListSortedAndFiltered(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	mustWrite("b.md", "b")
	mustWrite("a.markdown", "a")
	mustWrite("notes.txt", "ignored")
	mustWrite("nested/c.md", "c")
	mustWrite(".hidden/skip.md", "skip")

	got, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	wantRel := []string{"a.markdown", "b.md", filepath.Join("nested", "c.md")}
	if len(got) != len(wantRel) {
		t.Fatalf("got %d specs, want %d (%v)", len(got), len(wantRel), got)
	}
	for i, w := range wantRel {
		if got[i].Rel != w {
			t.Fatalf("specs[%d].Rel = %q, want %q", i, got[i].Rel, w)
		}
	}
}

package checkpoint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ralph.checkpoint")

	want := State{
		Prompt:        "build feature X",
		Model:         "gpt-x",
		WorkingDir:    "/tmp/x",
		PromisePhrase: "all done",
		Iteration:     5,
		MaxIterations: 10,
		LastSummary:   "summary",
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Prompt != want.Prompt || got.Iteration != want.Iteration || got.LastSummary != want.LastSummary {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.Version != CurrentVersion {
		t.Fatalf("expected version %d, got %d", CurrentVersion, got.Version)
	}
	if got.SavedAt.IsZero() {
		t.Fatalf("expected SavedAt to be populated")
	}
}

func TestLoadVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.json")
	if err := os.WriteFile(path, []byte(`{"version":999,"prompt":"x"}`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := Load(path)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestSaveAtomicNoTempLeftOver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.json")
	if err := Save(path, State{Prompt: "p"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected 1 file, got %v", names)
	}
}

func TestDeleteToleratesMissing(t *testing.T) {
	dir := t.TempDir()
	if err := Delete(filepath.Join(dir, "nope.json")); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	if err := Save(path, State{Prompt: "p"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := Delete(path); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected file removed, got %v", err)
	}
}

func TestSaveRejectsEmptyPath(t *testing.T) {
	if err := Save("", State{Prompt: "p"}); err == nil {
		t.Fatalf("expected error for empty path")
	}
}

func TestSaveReturnsMkdirError(t *testing.T) {
	dir := t.TempDir()
	parentFile := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}

	err := Save(filepath.Join(parentFile, "checkpoint.json"), State{Prompt: "p"})
	if err == nil {
		t.Fatalf("expected mkdir error")
	}
}

func TestSaveReturnsRenameError(t *testing.T) {
	dir := t.TempDir()
	err := Save(dir, State{Prompt: "p"})
	if err == nil {
		t.Fatalf("expected rename error when target is a directory")
	}
}

func TestLoadReportsReadAndParseErrors(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "missing.json"))
		if err == nil {
			t.Fatalf("expected read error")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, err := Load(path)
		if err == nil {
			t.Fatalf("expected parse error")
		}
	})
}

func TestDeleteEmptyPathIsNoop(t *testing.T) {
	if err := Delete(""); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDeleteReportsRemoveError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "child"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	err := Delete(dir)
	if err == nil {
		t.Fatalf("expected delete error for non-empty directory")
	}
}

// Package checkpoint persists Ralph loop state between runs so a halted
// loop can be resumed with `ralph resume <file>`.
//
// The format is intentionally simple JSON; older checkpoints written by
// future Ralph versions remain readable as long as new fields are added
// (never renamed or repurposed).
package checkpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CurrentVersion is the schema version written into every checkpoint.
const CurrentVersion = 1

// State is the serialised loop state.
type State struct {
	Version           int       `json:"version"`
	SavedAt           time.Time `json:"saved_at"`
	Prompt            string    `json:"prompt"`
	Model             string    `json:"model"`
	WorkingDir        string    `json:"working_dir"`
	PromisePhrase     string    `json:"promise_phrase,omitempty"`
	Iteration         int       `json:"iteration"`
	MaxIterations     int       `json:"max_iterations"`
	LastSummary       string    `json:"last_summary,omitempty"`
	ConsecutiveErrors int       `json:"consecutive_errors,omitempty"`
	ConsecNoChanges   int       `json:"consecutive_no_changes,omitempty"`
}

// ErrUnsupportedVersion is returned when Load encounters a checkpoint
// with a version Ralph does not understand.
var ErrUnsupportedVersion = errors.New("unsupported checkpoint version")

// Save writes the state atomically (write+rename) so a crash mid-write
// never leaves a partial file.
func Save(path string, s State) error {
	if path == "" {
		return errors.New("empty checkpoint path")
	}
	if s.Version == 0 {
		s.Version = CurrentVersion
	}
	if s.SavedAt.IsZero() {
		s.SavedAt = time.Now().UTC()
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temp checkpoint: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write checkpoint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close checkpoint: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename checkpoint: %w", err)
	}
	return nil
}

// Load reads and parses a checkpoint file.
func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, fmt.Errorf("read checkpoint %s: %w", path, err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("parse checkpoint %s: %w", path, err)
	}
	if s.Version != CurrentVersion {
		return State{}, fmt.Errorf("%w: %d (expected %d)", ErrUnsupportedVersion, s.Version, CurrentVersion)
	}
	return s, nil
}

// Delete removes a checkpoint file. Missing files are not an error.
func Delete(path string) error {
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("delete checkpoint %s: %w", path, err)
}

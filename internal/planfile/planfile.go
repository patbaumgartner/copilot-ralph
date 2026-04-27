// Package planfile manages Ralph's fix_plan.md scratchpad.
//
// fix_plan.md is the running TODO list the assistant maintains across
// iterations. The default location is `<working-dir>/.ralph/fix_plan.md`.
// The directory is created lazily on first write.
package planfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultRelPath is the default location of the plan file relative to the
// loop's working directory.
const DefaultRelPath = ".ralph/fix_plan.md"

// Resolve returns an absolute path for the plan file, defaulting to
// `<workingDir>/.ralph/fix_plan.md` when path is empty or relative.
func Resolve(workingDir, path string) string {
	if path == "" {
		path = DefaultRelPath
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workingDir, path)
}

// Read returns the current contents of the plan file. A missing file is
// reported as ("", nil) so callers can treat "not yet written" the same as
// "empty plan".
func Read(absPath string) (string, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read plan file: %w", err)
	}
	return string(data), nil
}

// EnsureDir creates the parent directory of absPath if it does not exist.
func EnsureDir(absPath string) error {
	dir := filepath.Dir(absPath)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create plan directory: %w", err)
	}
	return nil
}

// Snapshot is a stable representation of the plan file at one point in time.
// It is used to detect changes between iterations.
type Snapshot struct {
	Path    string
	Content string
	Exists  bool
}

// Take captures the current plan file state. A missing file is recorded as
// Exists=false with empty content.
func Take(absPath string) (Snapshot, error) {
	content, err := Read(absPath)
	if err != nil {
		return Snapshot{}, err
	}
	_, statErr := os.Stat(absPath)
	exists := statErr == nil
	return Snapshot{Path: absPath, Content: content, Exists: exists}, nil
}

// Changed reports whether two snapshots differ in existence or content.
func Changed(prev, next Snapshot) bool {
	if prev.Exists != next.Exists {
		return true
	}
	return prev.Content != next.Content
}

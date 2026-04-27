// Package specs enumerates Markdown specifications mounted into the loop
// system prompt via the --specs flag.
package specs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Spec describes a single Markdown spec file.
type Spec struct {
	// Path is the absolute filesystem path of the spec.
	Path string
	// Rel is the path relative to the user-supplied specs directory; this is
	// what we surface in prompts because it is shorter and stable.
	Rel string
	// Bytes is the raw file size on disk.
	Bytes int64
}

// List walks dir recursively and returns every `.md` / `.markdown` file in
// deterministic (alphabetical) order. dir is empty or unset returns
// (nil, nil) so callers can treat "no specs" as a non-error.
//
// Hidden directories (those starting with `.`) are skipped to avoid leaking
// `.git`, `.ralph`, and similar internal state into the prompt.
func List(dir string) ([]Spec, error) {
	if dir == "" {
		return nil, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("specs directory %s does not exist", dir)
		}
		return nil, fmt.Errorf("stat specs directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("specs path %s is not a directory", dir)
	}

	var out []Spec
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != dir && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = d.Name()
		}
		out = append(out, Spec{Path: path, Rel: rel, Bytes: fi.Size()})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk specs directory: %w", walkErr)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

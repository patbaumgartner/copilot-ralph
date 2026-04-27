// Package cli — `ralph doctor` checks the local environment for the
// dependencies Ralph relies on (Go binary's git, the Copilot CLI, and a
// writable working directory). It exits with a non-zero code when any
// required check fails.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/patbaumgartner/copilot-ralph/internal/tui/styles"
	"github.com/patbaumgartner/copilot-ralph/pkg/version"
)

var doctorCmd = &cobra.Command{
	Use:           "doctor",
	Short:         "Check the local environment for required tools",
	Long:          `Verify git, the Copilot CLI, and the working directory are usable.`,
	RunE:          runDoctor,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// check is one diagnostic line.
type check struct {
	Name    string
	Status  string
	Detail  string
	Ok      bool
	Warn    bool
	Skipped bool
}

func runDoctor(cmd *cobra.Command, args []string) error {
	_ = cmd
	_ = args

	checks := []check{
		checkRalphVersion(),
		checkBinary("git"),
		checkBinary("copilot"),
		checkWorkingDir(),
	}

	failed := false
	for _, c := range checks {
		marker := styles.SuccessStyle.Render("✓")
		if c.Warn {
			marker = styles.WarningStyle.Render("!")
		}
		if !c.Ok && !c.Warn {
			marker = styles.ErrorStyle.Render("✗")
			failed = true
		}
		line := fmt.Sprintf("%s %-20s %s", marker, c.Name, c.Status)
		if c.Detail != "" {
			line = fmt.Sprintf("%s — %s", line, c.Detail)
		}
		fmt.Println(line)
	}

	if failed {
		return &ExitError{Code: exitFailed, Err: fmt.Errorf("one or more required checks failed")}
	}
	return nil
}

func checkRalphVersion() check {
	return check{
		Name:   "ralph",
		Status: version.Version,
		Ok:     true,
	}
}

// checkBinary verifies the named binary is on PATH and returns its
// reported version (best-effort: tries `--version` first, then `version`).
func checkBinary(name string) check {
	path, err := exec.LookPath(name)
	if err != nil {
		c := check{Name: name, Status: "missing", Detail: err.Error()}
		// copilot is required for `run`; git is required for auto-commit.
		// We always treat both as required so doctor surfaces clear gaps.
		c.Ok = false
		return c
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, "--version").CombinedOutput()
	status := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if err != nil || status == "" {
		// Try `name version` as a fallback (some CLIs use that form).
		out2, err2 := exec.CommandContext(ctx, name, "version").CombinedOutput()
		if err2 == nil {
			status = strings.TrimSpace(strings.SplitN(string(out2), "\n", 2)[0])
		}
	}
	if status == "" {
		status = path
	}
	return check{Name: name, Status: status, Ok: true}
}

func checkWorkingDir() check {
	wd, err := os.Getwd()
	if err != nil {
		return check{Name: "working-dir", Status: "unreadable", Detail: err.Error()}
	}
	info, err := os.Stat(wd)
	if err != nil || !info.IsDir() {
		return check{Name: "working-dir", Status: wd, Detail: "not a directory", Ok: false}
	}
	// Probe writability by creating a tiny temp file.
	f, err := os.CreateTemp(wd, ".ralph-doctor-")
	if err != nil {
		return check{Name: "working-dir", Status: wd, Detail: "not writable: " + err.Error(), Ok: false}
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
	return check{Name: "working-dir", Status: wd, Ok: true}
}

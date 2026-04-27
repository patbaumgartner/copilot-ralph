package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patbaumgartner/copilot-ralph/internal/sdk"
)

func TestBuildIterationPromptWithoutCarryContext(t *testing.T) {
	cfg := DefaultLoopConfig()
	cfg.Prompt = "do the thing"
	cfg.MaxIterations = 3
	e := NewLoopEngine(cfg, nil)
	got := e.buildIterationPrompt(1)
	if !strings.Contains(got, "[Iteration 1/3]") {
		t.Fatalf("missing iteration header: %q", got)
	}
	if !strings.HasSuffix(got, "do the thing") {
		t.Fatalf("expected prompt to end with task, got %q", got)
	}
	if strings.Contains(got, "Previous iteration summary") {
		t.Fatalf("did not expect carry-context section, got %q", got)
	}
}

func TestBuildIterationPromptCarriesSummary(t *testing.T) {
	cfg := DefaultLoopConfig()
	cfg.Prompt = "task"
	e := NewLoopEngine(cfg, nil)
	e.lastSummary = "did A; will do B"
	got := e.buildIterationPrompt(2)
	if !strings.Contains(got, "Previous iteration summary:\ndid A; will do B") {
		t.Fatalf("missing carry-context section: %q", got)
	}
}

func TestBuildIterationPromptIncludesPlanFile(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, ".ralph", "fix_plan.md")
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(planPath, []byte("- [ ] step 1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := DefaultLoopConfig()
	cfg.Prompt = "task"
	cfg.WorkingDir = dir
	cfg.PlanFile = ".ralph/fix_plan.md"
	e := NewLoopEngine(cfg, nil)

	got := e.buildIterationPrompt(1)
	if !strings.Contains(got, "Running plan ("+planPath+")") {
		t.Fatalf("missing plan section: %q", got)
	}
	if !strings.Contains(got, "- [ ] step 1") {
		t.Fatalf("missing plan content: %q", got)
	}
}

func TestBuildIterationPromptIncludesSpecs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("# A"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.md"), []byte("# B"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := DefaultLoopConfig()
	cfg.Prompt = "task"
	cfg.SpecsDir = dir
	e := NewLoopEngine(cfg, nil)
	got := e.buildIterationPrompt(1)
	if !strings.Contains(got, "Available specs under "+dir+":") {
		t.Fatalf("missing specs header: %q", got)
	}
	if !strings.Contains(got, "alpha.md") || !strings.Contains(got, "beta.md") {
		t.Fatalf("missing spec entries: %q", got)
	}
}

func TestUpdateCarryContextSummary(t *testing.T) {
	cfg := DefaultLoopConfig()
	cfg.CarryContext = CarryContextSummary
	cfg.CarryContextMaxRunes = 0
	e := NewLoopEngine(cfg, nil)

	e.updateCarryContext("noise <summary>first</summary>")
	if e.lastSummary != "first" {
		t.Fatalf("got %q want %q", e.lastSummary, "first")
	}

	// No summary in next response keeps previous summary.
	e.updateCarryContext("just text without tags")
	if e.lastSummary != "first" {
		t.Fatalf("expected previous summary preserved, got %q", e.lastSummary)
	}

	// New summary replaces previous.
	e.updateCarryContext("<summary>second</summary>")
	if e.lastSummary != "second" {
		t.Fatalf("got %q want %q", e.lastSummary, "second")
	}
}

func TestUpdateCarryContextOff(t *testing.T) {
	cfg := DefaultLoopConfig()
	cfg.CarryContext = CarryContextOff
	e := NewLoopEngine(cfg, nil)
	e.lastSummary = "should be cleared"
	e.updateCarryContext("<summary>ignored</summary>")
	if e.lastSummary != "" {
		t.Fatalf("expected empty summary, got %q", e.lastSummary)
	}
}

func TestUpdateCarryContextVerbatim(t *testing.T) {
	cfg := DefaultLoopConfig()
	cfg.CarryContext = CarryContextVerbatim
	cfg.CarryContextMaxRunes = 0
	e := NewLoopEngine(cfg, nil)
	e.updateCarryContext("  full body  ")
	if e.lastSummary != "full body" {
		t.Fatalf("expected trimmed verbatim, got %q", e.lastSummary)
	}
}

// errorMockSDK emits an ErrorEvent on every prompt. Used to exercise the
// stop-on-error counter.
type errorMockSDK struct {
	model   string
	started bool
}

func newErrorMockSDK() *errorMockSDK { return &errorMockSDK{model: "err"} }

func (m *errorMockSDK) Start() error                             { m.started = true; return nil }
func (m *errorMockSDK) Stop() error                              { m.started = false; return nil }
func (m *errorMockSDK) CreateSession(ctx context.Context) error  { return nil }
func (m *errorMockSDK) DestroySession(ctx context.Context) error { return nil }
func (m *errorMockSDK) Model() string                            { return m.model }
func (m *errorMockSDK) SendPrompt(ctx context.Context, p string) (<-chan sdk.Event, error) {
	ch := make(chan sdk.Event, 2)
	go func() {
		defer close(ch)
		ch <- sdk.NewErrorEvent(context.DeadlineExceeded)
	}()
	return ch, nil
}

func TestStopOnErrorThreshold(t *testing.T) {
	cfg := DefaultLoopConfig()
	cfg.Prompt = "x"
	cfg.MaxIterations = 10
	cfg.StopOnError = 2
	cfg.WorkingDir = t.TempDir()

	engine := NewLoopEngine(cfg, newErrorMockSDK())

	go func() {
		for range engine.Events() { //nolint:revive
		}
	}()

	result, err := engine.Start(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if result == nil || result.State != StateFailed {
		t.Fatalf("expected failed state, got %+v", result)
	}
	if !strings.Contains(err.Error(), "consecutive error") {
		t.Fatalf("expected consecutive-error message, got %v", err)
	}
	if result.Iterations != 2 {
		t.Fatalf("expected to stop after 2 iterations, got %d", result.Iterations)
	}
}

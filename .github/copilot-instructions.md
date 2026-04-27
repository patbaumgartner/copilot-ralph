# Ralph Development Guide

This file is the single source of truth for AI coding agents and human
contributors working on Ralph. It is read automatically by GitHub Copilot Chat
in VS Code.

Human contributors should also read [CONTRIBUTING.md](../CONTRIBUTING.md) for
day-to-day workflow.

## What Ralph is

Ralph is a small Go CLI that drives an iterative loop against the GitHub
Copilot SDK. The user provides a prompt; Ralph re-sends it (with the previous
response folded in) until one of the following conditions is met:

- the assistant signals completion by emitting `<promise>...</promise>`,
- the configured `--max-iterations` is reached,
- the `--timeout` or `--iteration-timeout` elapses,
- an unrecoverable SDK error occurs, or
- the user sends `Ctrl+C` (or `Ctrl+\` when a checkpoint is active).

There is **no** Bubble Tea TUI, **no** wizard, **no** `ralph init` command,
**no** `/specs` directory. Output is plain streamed text styled with
`lipgloss`.

## Technology stack

- **Go 1.26+** (see `go.mod` for the exact toolchain).
- **CLI:** `github.com/spf13/cobra`.
- **Styling:** `github.com/charmbracelet/lipgloss`.
- **AI:** `github.com/github/copilot-sdk/go` v0.3.x.
- **Tests:** `github.com/stretchr/testify`.

## Repository layout

```text
cmd/ralph/                Entry point. Translates *cli.ExitError to an
                          os.Exit code; defers nothing fancy.
internal/cli/             Cobra commands (run, resume, reset, doctor,
                          version), flag wiring, ExitError type, prompt
                          resolution.
internal/core/            Loop engine, state machine, event types,
                          promise detector, embedded system prompt.
internal/sdk/             Copilot SDK client wrapper, session lifecycle,
                          retry/backoff, rate-limit handling, event
                          translation.
internal/checkpoint/      Atomic loop-state persistence for ralph resume.
internal/eventsink/       Fan-out delivery of events to JSON, log file,
                          and webhook sinks.
internal/gitutil/         git helpers: commit, tag, diff-stat, clean check.
internal/oracle/          Second-opinion Copilot client (streaming off).
internal/planfile/        Atomic read/write of the fix_plan.md scratchpad.
internal/specs/           Enumeration of a spec directory.
internal/verify/          Shell-command runner with timeout and output cap.
internal/tui/styles/      lipgloss styles + Ralph ASCII art splash.
pkg/version/              Version metadata populated via -ldflags.
```

Dependency direction is strictly **CLI → core → SDK** plus any of the
internal utility packages. The SDK package has no internal dependencies. Do
not introduce cycles.

## Commands

| Command         | Purpose                                                        |
| --------------- | -------------------------------------------------------------- |
| `ralph run`     | Run the iteration loop with the given prompt.                  |
| `ralph resume`  | Resume from a `--checkpoint-file` written by a prior run.      |
| `ralph reset`   | Delete a checkpoint file (`--force` skips confirmation).       |
| `ralph doctor`  | Run environment health checks (Copilot CLI, git, writable cwd).|
| `ralph version` | Print build metadata.                                          |

## Key flags (non-exhaustive)

See `ralph run --help` for the full list and defaults.

| Category              | Flag                        | Description                                  |
| --------------------- | --------------------------- | -------------------------------------------- |
| Loop limits           | `--max-iterations`          | Stop after N loops.                          |
|                       | `--timeout`                 | Stop after this duration.                    |
|                       | `--iteration-timeout`       | Per-iteration soft deadline.                 |
| Stop conditions       | `--stop-on-no-changes`      | Halt after N clean-tree iterations.          |
|                       | `--stop-on-error`           | Halt after N error iterations.               |
| Carry context         | `--carry-context`           | `off` / `summary` / `verbatim`.              |
|                       | `--carry-context-max-runes` | Max runes carried forward.                   |
| Prompt engineering    | `--prompt-stack`            | Extra Markdown files prepended each turn.    |
|                       | `--plan-file`               | Shared fix-plan scratchpad.                  |
|                       | `--specs`                   | Directory of spec Markdown files.            |
| Build verification    | `--verify-cmd`              | Shell command run after each iteration.      |
|                       | `--verify-timeout`          | Timeout for a single verify run.             |
|                       | `--verify-max-bytes`        | Max bytes captured per verify run.           |
| Auto-commit           | `--auto-commit`             | `git add -A && commit` after each iteration. |
|                       | `--auto-commit-message`     | Format string (`%d` = iteration number).     |
|                       | `--auto-commit-on-failure`  | Commit even when verify failed.              |
|                       | `--auto-tag`                | Annotated tag format for auto-commits.       |
|                       | `--diff-stat`               | Emit `git diff --stat HEAD` each iteration.  |
| Output sinks          | `--json`                    | Emit JSON Lines to stdout.                   |
|                       | `--json-output`             | Also write JSON Lines to a file.             |
|                       | `--log-file`                | Append one-line text summaries.              |
|                       | `--webhook`                 | POST each event as JSON.                     |
|                       | `--webhook-timeout`         | Timeout for a single webhook delivery.       |
| Persistence           | `--checkpoint-file`         | Persist state for `ralph resume`.            |
| Oracle                | `--oracle-model`            | Second-opinion model name.                   |
|                       | `--oracle-every`            | Consult every N iterations.                  |
|                       | `--oracle-on-verify-fail`   | Consult whenever verify fails.               |
| Rate limiting         | `--no-rate-limit-wait`      | Fail fast instead of waiting for reset.      |
| Model / session       | `--model`                   | Copilot model id.                            |
|                       | `--streaming`               | Stream deltas vs. full messages.             |
|                       | `--system-prompt`           | Inline text or path to a Markdown file.      |
|                       | `--system-prompt-mode`      | `append` or `replace` Ralph's system prompt. |
|                       | `--dry-run`                 | Print resolved config and exit.              |

## Architectural conventions

### Spec-by-code

There is no `/specs` directory. The code, package doc comments, and tests
are the specification. When behaviour changes, update the package doc and
the tests in the same PR.

### Interface-based seams

Cross-package boundaries use interfaces so they can be mocked. Example: the
loop engine consumes an `SDKClient` interface, not the concrete
`*sdk.CopilotClient`.

### Dependency injection

Dependencies are passed explicitly through constructors. No package-level
mutable globals beyond Cobra flag bindings (which are unavoidable due to
the Cobra API). Flag pointers are copied into a `core.LoopConfig` value
before any business logic runs.

### Event-driven progress

The loop engine emits typed events on a channel. The CLI subscribes and
prints them. Additional consumers (metrics, JSON output, webhook) plug into
`internal/eventsink`'s `FanOut`; do not reach into engine internals.

## Go conventions

### No `else`

Always use early returns.

```go
// Good
if err != nil {
    return fmt.Errorf("read config: %w", err)
}
return process(cfg)

// Bad
if err != nil {
    return err
} else {
    return process(cfg)
}
```

### Wrap errors with `%w`

Every error crossing a package boundary gets context. Never `return err`
without thinking about it.

### Check errors immediately

Never `_ =`-assign an error unless you have a documented reason in a
comment on the same line.

### Document exported symbols

A one-line `// Foo does X.` is enough. Package-level doc comments belong
on a `doc.go` file or the most prominent file in the package.

### Table-driven tests

Anything with more than one interesting input gets a slice of test cases.

```go
tests := []struct {
    name string
    in   string
    want bool
}{
    {"plain", "<promise>done</promise>", true},
    {"missing", "almost done", false},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got := detectPromise(tt.in, "done")
        require.Equal(t, tt.want, got)
    })
}
```

## Copilot SDK integration

The SDK has a tagged-union `SessionEventData` interface. Concrete types
relevant to Ralph (non-exhaustive):

- `*copilot.AssistantMessageDeltaData` — streaming text chunks.
- `*copilot.AssistantMessageData` — final message text.
- `*copilot.AssistantReasoningDeltaData` / `*copilot.AssistantReasoningData`
  — reasoning trace.
- `*copilot.ToolExecutionStartData` — assistant invoking a tool.
- `*copilot.ToolExecutionCompleteData` — tool result (`Success` is `bool`,
  not `*bool`; `ToolName` is only on the start event).
- `*copilot.SessionIdleData` — session ready for the next prompt.
- `*copilot.SessionErrorData` — fatal session error.

When adding new event handling, switch on the concrete type. Keep SDK types
confined to `internal/sdk`; expose Ralph-specific event structs to the rest
of the codebase.

## Error handling

The CLI must never call `os.Exit` from inside a Cobra `RunE`. That bypasses
deferred SDK shutdown. Instead, return a `*cli.ExitError{Code, Err}`; the
`main` function in `cmd/ralph` translates it to a process exit code.

```go
return &ExitError{Code: ExitCodeTimeout, Err: result.Err}
```

For retries, prefer typed predicates over string matching with bounded
exponential backoff. The SDK client already implements this for transient
session errors.

## Testing

- Unit tests live next to the code they cover (`foo.go` ↔ `foo_test.go`).
- Tests that need a real Copilot CLI must call `skipIfNoSDK(t)` (or an
  equivalent guard) so CI skips them gracefully.
- New code requires tests. Bug fixes require regression tests.

Run locally:

```bash
make test    # go test -race -cover ./...
make vet     # go vet ./...
make lint    # golangci-lint (must be built with the same Go toolchain)
make build   # go build ./...
make all     # tidy + fmt + vet + lint + test + build
```

## Adding things

### A new CLI flag

1. Declare the variable in `internal/cli/run.go` next to the existing flags.
2. Bind it with `cmd.Flags().*Var*` inside `registerRunFlags`.
3. Validate it inside `validateSettings(cfg *core.LoopConfig)`.
4. Copy it into `core.LoopConfig` so business logic does not read package
   globals.
5. Add table-driven tests in `internal/cli/cli_test.go`.

### A new loop event

1. Add a typed struct in `internal/core/events.go`.
2. Emit it from the engine where the new condition occurs.
3. Handle it in `internal/cli/run.go`'s event listener.
4. Cover both with tests.

### A new output sink

1. Implement the `eventsink.Sink` interface in `internal/eventsink/`.
2. Register the sink in `internal/cli/run.go` alongside the existing sinks.
3. Add unit tests for the sink in isolation.

## Documentation hygiene

- Update [CHANGELOG.md](../CHANGELOG.md) under `[Unreleased]` for any
  user-visible change.
- Keep README.md user-facing. Keep this file developer-facing.
- If you remove a feature, grep the repo for stale references before
  shipping.

## Pre-merge checklist

- [ ] `make all` is green.
- [ ] New behaviour is covered by tests.
- [ ] CHANGELOG updated.
- [ ] No new `else` blocks, no unwrapped errors, no `os.Exit` from `RunE`.
- [ ] Public symbols are documented.

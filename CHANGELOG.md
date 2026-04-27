# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.2] - 2026-04-27

### Fixed

- Light-background terminal colors now correctly detected in VS Code integrated
  terminals. `VSCODE_THEME_KIND` is now checked first (always set by VS Code)
  before the `COLORFGBG` fallback, so the muted deep-color palette is applied
  reliably when a light or high-contrast-light theme is active.
- Light-background palette colors darkened to meet WCAG AA contrast (≥ 4.5:1
  against white): Primary → Deep Violet, Success → Forest Green,
  Warning → Burnt Amber, Info → Navy Blue.

### Changed

- Test coverage improved: added tests for `ExitError`, `exitErrorFor`,
  `ralph doctor` checks, `ralph reset` command, and all style/theme-detection
  paths. `internal/tui/styles` reaches 100% statement coverage.

## [0.2.1] - 2026-04-27

### Fixed

- `ralph --version` and `ralph version` now correctly display the module
  version (e.g. `0.2.1`) when installed via `go install
  github.com/patbaumgartner/copilot-ralph/cmd/ralph@v0.2.1`. Previously the
  version reported was always `dev` because the `debug.ReadBuildInfo()`
  fallback was not present in the module-proxy-cached v0.2.0 zip, and the
  `v` prefix from the embedded build info was not stripped.
- Terminal output colors adapt to the terminal background: a dark-background
  palette (vibrant/neon) is used by default; a muted deep-color palette is
  applied automatically when `COLORFGBG` indicates a light background.

## [0.2.0] - 2026-04-27

### Added

#### Env-var overrides

- All major flags now read a `RALPH_*` environment variable as their default
  so Ralph can be configured without repeating flags on every invocation.
  Supported variables: `RALPH_MAX_ITERATIONS`, `RALPH_TIMEOUT`,
  `RALPH_ITERATION_TIMEOUT`, `RALPH_PROMISE`, `RALPH_MODEL`,
  `RALPH_WORKING_DIR`, `RALPH_STREAMING`, `RALPH_SYSTEM_PROMPT`,
  `RALPH_CARRY_CONTEXT`, `RALPH_NO_RATE_LIMIT_WAIT`, `RALPH_VERIFY_CMD`,
  `RALPH_CHECKPOINT_FILE`, `RALPH_ORACLE_MODEL`, `RALPH_BLOCKED_PHRASE`,
  `RALPH_STALL_AFTER`, `RALPH_ITERATION_DELAY`, `RALPH_ON_COMPLETE`,
  `RALPH_ON_BLOCKED`.

#### Shell completions

- `ralph completion <shell>` — generate shell completion scripts for
  `bash`, `zsh`, `fish`, and `powershell`.

#### Blocked signal

- `--blocked-phrase` — when non-empty, the engine watches every assistant
  response for `<blocked>...</blocked>` wrapping that phrase. When detected,
  the loop stops immediately with a new **exit code 5** (`StateBlocked`) and a
  `BlockedPhraseDetectedEvent` is emitted. The `--on-blocked` hook fires.
  Set `RALPH_BLOCKED_PHRASE` to configure without flags.

#### Stall detection

- `--stall-after N` — halt the loop after N consecutive iterations that
  produce byte-for-byte identical assistant responses (whitespace trimmed).
  Protects against models stuck in a repeating non-progressing loop.
  Default `0` disables. Set `RALPH_STALL_AFTER` to configure without flags.

#### Iteration delay

- `--iteration-delay` — configurable pause inserted between iterations.
  Useful for pacing API calls or giving external systems time to settle.
  Respects context cancellation. Default `0` disables.
  Set `RALPH_ITERATION_DELAY` to configure without flags.

#### Stdin prompt

- `ralph run -` — pass `-` as the prompt argument to read the prompt from
  standard input. Enables pipeline usage:
  `echo "fix the linter" | ralph run -`

#### Lifecycle hooks

- `--on-complete <cmd>` — shell command executed after the loop completes
  successfully (promise detected). The hook receives `RALPH_STATE` and
  `RALPH_ITERATIONS` environment variables. Errors are printed as warnings
  and do not alter Ralph's exit code. Set `RALPH_ON_COMPLETE` to configure
  without flags.
- `--on-blocked <cmd>` — same as `--on-complete` but runs when the model
  emits the blocked signal. Set `RALPH_ON_BLOCKED` to configure without flags.

## [0.1.0] - 2026-04-27

### Added

#### Core loop

- Initial release of Ralph — a small Go CLI that drives an iterative loop
  against the [GitHub Copilot SDK](https://github.com/github/copilot-sdk).
- `ralph run <prompt>`: send a prompt (inline text or path to a Markdown file)
  and iterate until the model signals completion, a hard limit is reached, or
  the user interrupts with `Ctrl+C`.
- Promise-based completion: the loop ends cleanly when the assistant emits
  `<promise>...</promise>`. The phrase inside the tag is configurable with
  `--promise`.
- Streaming output: assistant tokens, reasoning traces, and tool calls land on
  stdout as they arrive. Disable with `--streaming=false` to wait for the full
  message instead.
- ANSI-styled terminal output with 24-bit colour — no external TUI library
  required.
- Clean exit codes: `0` finished, `1` SDK/generic error, `2` cancelled,
  `3` timeout, `4` max-iterations reached.

#### Loop control

- `--max-iterations` / `-m`: stop after N loops (default: 10).
- `--timeout` / `-t`: hard wall-clock deadline for the whole run (default: 30m).
- `--iteration-timeout`: per-iteration soft deadline; the assistant is asked to
  wrap up when the deadline is close.
- `--stop-on-no-changes`: halt after N consecutive iterations with no git
  working-tree changes.
- `--stop-on-error`: halt after N consecutive iterations that emit at least one
  error event.
- `--working-dir`: the directory where the assistant runs tools (default: cwd).
- `--model`: Copilot model id (default: `gpt-4`).
- `--log-level`: verbosity (`debug` / `info` / `warn` / `error`).
- `--dry-run`: print the fully resolved configuration and exit without calling
  the model.

#### Prompt engineering

- `--system-prompt`: supply a custom system message as inline text or a path to
  a Markdown file.
- `--system-prompt-mode`: `append` (default) prepends the custom prompt before
  Ralph's built-in instructions; `replace` swaps them out entirely.
- `--carry-context`: fold the previous response back into the next prompt as
  `off`, `summary` (default), or `verbatim`.
- `--carry-context-max-runes`: cap the amount of prior context carried forward
  (default: 4 000 runes).
- `--prompt-stack`: one or more additional prompts (paths or literals) prepended
  to the main prompt on every iteration.
- `--plan-file`: path to a shared Markdown scratchpad (`fix_plan.md`); its
  content is injected into every prompt so the model can maintain a running
  plan across iterations.
- `--specs`: directory of Markdown spec files listed in every prompt to give
  the model persistent context about requirements.

#### Build verification

- `--verify-cmd`: shell command run after each iteration; a non-zero exit folds
  the captured output into the next prompt as feedback.
- `--verify-timeout`: per-run timeout for `--verify-cmd` (default: 5 min).
- `--verify-max-bytes`: cap on bytes captured per stream from `--verify-cmd`
  (default: 16 KiB).

#### Auto-commit & git integration

- `--auto-commit`: run `git add -A && git commit` after each iteration that
  produced changes (never pushes).
- `--auto-commit-message`: format string for the commit message; `%d` is
  replaced with the iteration number (default: `ralph: iteration %d`).
- `--auto-commit-on-failure`: commit even when `--verify-cmd` failed.
- `--auto-tag`: format string for an annotated tag created on each
  auto-commit (e.g. `ralph/iter-%d`).
- `--diff-stat`: emit `git diff --stat HEAD` as an event after each iteration.

#### Output sinks

- `--json`: emit JSON Lines envelopes to stdout instead of styled output.
- `--json-output`: write JSON Lines to a file in addition to styled stdout.
- `--log-file`: append a one-line human-readable summary of every event to a
  file.
- `--webhook`: POST every event as JSON to a URL.
- `--webhook-timeout`: per-delivery timeout for `--webhook` (default: 5 s).

#### Checkpoint & resume

- `--checkpoint-file`: persist loop state atomically after every iteration.
- `ralph resume`: pick up a prior run exactly where it left off using a
  checkpoint file.
- `ralph reset`: delete a checkpoint file (`--force` skips confirmation).

#### Oracle (second opinion)

- `--oracle-model`: name of a second Copilot model consulted between
  iterations for an independent assessment.
- `--oracle-every`: consult the oracle every N iterations.
- `--oracle-on-verify-fail`: automatically consult the oracle whenever
  `--verify-cmd` fails.

#### Rate limiting & reliability

- Exponential backoff on Copilot rate-limit responses by default.
- `--no-rate-limit-wait`: fail immediately on rate-limit / quota errors
  instead of waiting for the reset window.

#### Other commands

- `ralph doctor`: run environment health checks (Copilot CLI on `$PATH`, git
  available, working directory writable).
- `ralph version`: print build metadata (version, commit, build date,
  Go version).

[Unreleased]: https://github.com/patbaumgartner/copilot-ralph/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/patbaumgartner/copilot-ralph/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/patbaumgartner/copilot-ralph/releases/tag/v0.1.0

# Ralph

```text
              --.                        .-+.
              -+++--.                 .-+++#.
              +++++++-.            .-+++++++-
              +++++++++-. .....   .+++++++++-
              +++++++++++-++++++--++++++++++-
              ++++++++++++++++++++++++++++++-
              +++++++++++++++++++----+++++++-
              -++++++++++++++++++.   ..--+++.
              .+++++-...--+++++++     ...-+-
               -+++-      ..---++...   ...+.
               .++++.     +#+ -++.+#+ ...--
                -++++..   -++.-++-+#-...-+.
                  -++++-     .###-  .-++.
                    .-+.       .     .+
                     .+-.   --.    ...
                    .++++       ..-+-. ..
                    .++++      ...-.-+--.
                    .++++-.    +#+-..-----
                    -+++++++--######++-- .
       ...          -++++---++#####+++-+-.
       ..     .-.   -++++.    ...-+--...
        .    .-++--..+++++-.. ...-+++-
        ...  .-+++++-+++++++----+++++.
          ...++++++++++++--....--+++-
           .-+++++++++++-        .++.
         .......-----##+..........+##-.....
              ........................
```

> A small, opinionated Go CLI that drives an iterative loop against the
> [GitHub Copilot SDK](https://github.com/github/copilot-sdk). One prompt
> in, many turns of progress out — until Ralph decides he's done, you run
> out of iterations, or you hit `Ctrl+C`.

[![CI](https://github.com/patbaumgartner/copilot-ralph/actions/workflows/ci.yml/badge.svg)](https://github.com/patbaumgartner/copilot-ralph/actions/workflows/ci.yml)
[![Release](https://github.com/patbaumgartner/copilot-ralph/actions/workflows/release.yml/badge.svg)](https://github.com/patbaumgartner/copilot-ralph/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/patbaumgartner/copilot-ralph.svg)](https://pkg.go.dev/github.com/patbaumgartner/copilot-ralph)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Why Ralph

The "Ralph Wiggum" technique is dead simple: keep poking the model with the
same prompt until the work is actually done. Ralph wraps that idea in a
single binary so you can hand a task to Copilot and watch it iterate
without babysitting the chat window.

- **Stream first.** Tokens, reasoning, and tool calls land on stdout the
  moment they arrive.
- **Promise-based completion.** The model wraps its sign-off in
  `<promise>...</promise>`; Ralph ends the loop cleanly when it sees one.
- **Hard limits.** `--max-iterations` and `--timeout` keep runaway loops
  in check.
- **Verifiable progress.** `--verify-cmd` runs your build/test suite after
  every iteration and feeds failures back as context for the next turn.
- **Auto-commit.** `--auto-commit` snapshots each iteration as a git commit,
  optionally tagged, so you can bisect if something goes wrong.
- **Second opinion.** `--oracle-model` consults a second Copilot model
  between iterations for an independent assessment.
- **No magic.** Plain CLI, plain logs, no TUI, no wizard.

## Install

Requires Go 1.26+ and the [GitHub Copilot CLI](https://github.com/github/copilot)
on `$PATH`.

```bash
# Install the latest release directly
go install github.com/patbaumgartner/copilot-ralph/cmd/ralph@latest

# Or grab a binary from the Releases page
# https://github.com/patbaumgartner/copilot-ralph/releases

# Or build from source
git clone https://github.com/patbaumgartner/copilot-ralph.git
cd copilot-ralph
make build   # produces ./bin/ralph
```

## Quick start

```bash
# Inline prompt — Ralph iterates until the model promises it's done
ralph run "Add unit tests for the parser module"

# Markdown file as prompt
ralph run task.md

# Cap iterations and runtime
ralph run --max-iterations 5 --timeout 10m "Refactor the auth module"

# Run your test suite after every iteration; feed failures back automatically
ralph run --verify-cmd "make test" "Fix all failing tests"

# Auto-commit each iteration and run the oracle every 3 turns
ralph run --auto-commit --oracle-model gpt-4o --oracle-every 3 "Optimise hot path"

# Show the resolved config without calling the model
ralph run --dry-run "Implement OAuth"
```

## Features

### Loop control

| Flag | Default | Purpose |
| ---- | ------- | ------- |
| `-m`, `--max-iterations` | `10` | Stop after N loops. |
| `-t`, `--timeout` | `30m` | Hard wall-clock deadline. |
| `--iteration-timeout` | `0` | Per-iteration soft deadline (0 disables). |
| `--stop-on-no-changes` | `0` | Halt after N iterations with no git changes. |
| `--stop-on-error` | `0` | Halt after N iterations emitting errors. |
| `--model` | `gpt-4` | Copilot model id. |
| `--working-dir` | cwd | Directory where the assistant runs tools. |
| `--log-level` | `info` | `debug` / `info` / `warn` / `error`. |
| `--dry-run` | `false` | Print resolved config and exit. |

### Prompt & context

| Flag | Default | Purpose |
| ---- | ------- | ------- |
| `--promise` | `I'm special!` | Phrase the model wraps in `<promise>`. |
| `--streaming` | `true` | Stream deltas vs. wait for full messages. |
| `--system-prompt` | (built-in) | Inline text or path to a Markdown file. |
| `--system-prompt-mode` | `append` | `append` or `replace` Ralph's prompt. |
| `--carry-context` | `summary` | `off` / `summary` / `verbatim`. |
| `--carry-context-max-runes` | `4000` | Max runes carried into the next prompt. |
| `--prompt-stack` | (none) | Extra prompts prepended each iteration. |
| `--plan-file` | (none) | Shared Markdown scratchpad injected every turn. |
| `--specs` | (none) | Directory of spec files listed each turn. |

### Build verification

| Flag | Default | Purpose |
| ---- | ------- | ------- |
| `--verify-cmd` | (none) | Shell command run after each iteration. |
| `--verify-timeout` | `5m` | Timeout for a single verify run. |
| `--verify-max-bytes` | `16384` | Max bytes captured per stream. |

### Auto-commit & git

| Flag | Default | Purpose |
| ---- | ------- | ------- |
| `--auto-commit` | `false` | `git add -A && commit` after each iteration. |
| `--auto-commit-message` | `ralph: iteration %d` | Commit message format (`%d` = iteration). |
| `--auto-commit-on-failure` | `false` | Commit even when verify failed. |
| `--auto-tag` | (none) | Annotated tag format (e.g. `ralph/iter-%d`). |
| `--diff-stat` | `false` | Emit `git diff --stat HEAD` each iteration. |

### Output sinks

| Flag | Default | Purpose |
| ---- | ------- | ------- |
| `--json` | `false` | Emit JSON Lines to stdout. |
| `--json-output` | (none) | Also write JSON Lines to a file. |
| `--log-file` | (none) | Append a one-line summary of every event. |
| `--webhook` | (none) | POST every event as JSON to this URL. |
| `--webhook-timeout` | `5s` | Timeout for a single webhook delivery. |

### Checkpoint & resume

| Flag | Default | Purpose |
| ---- | ------- | ------- |
| `--checkpoint-file` | (none) | Persist loop state after every iteration. |

```bash
# Pause a long run with Ctrl+\ and resume later
ralph run --checkpoint-file state.json "Big refactor"
ralph resume --checkpoint-file state.json
```

### Oracle (second opinion)

| Flag | Default | Purpose |
| ---- | ------- | ------- |
| `--oracle-model` | (none) | Second Copilot model consulted between iters. |
| `--oracle-every` | `0` | Consult every N iterations (0 disables). |
| `--oracle-on-verify-fail` | `false` | Consult whenever verify fails. |

### Rate limiting

| Flag | Default | Purpose |
| ---- | ------- | ------- |
| `--no-rate-limit-wait` | `false` | Fail fast instead of waiting for reset. |

## Commands

| Command | Purpose |
| ------- | ------- |
| `ralph run <prompt>` | Run the iteration loop. |
| `ralph resume` | Resume from a `--checkpoint-file`. |
| `ralph reset` | Delete a checkpoint file (`--force` skips confirmation). |
| `ralph doctor` | Check environment health (Copilot CLI, git, writable cwd). |
| `ralph version` | Print build metadata. |

## How it works

```text
┌─────────────────────────────────────────────────────────────┐
│  ralph run "Your task"                                      │
│                                                             │
│  1. Build prompt ← system prompt + plan-file + specs +      │
│                    previous response (carry-context) + user │
│  2. Send to Copilot SDK (streaming)                         │
│  3. Print assistant tokens, reasoning, and tool events      │
│  4. Run --verify-cmd (if set) and capture output            │
│  5. Optionally consult oracle model for second opinion      │
│  6. Auto-commit (if --auto-commit)                          │
│  7. Check stop conditions:                                  │
│       • <promise>...</promise> detected → done ✓            │
│       • --max-iterations reached → exit 4                   │
│       • --timeout elapsed → exit 3                          │
│       • --stop-on-no-changes or --stop-on-error triggered   │
│  8. Loop back to step 1 with the next iteration             │
└─────────────────────────────────────────────────────────────┘
```

The loop engine lives in `internal/core`. The SDK wrapper (`internal/sdk`)
handles sessions, retries, and rate-limit backoff. The CLI layer
(`cmd/ralph`, `internal/cli`) wires flags to a `LoopConfig` struct and
prints typed events as they stream in. Output sinks (JSON, log file,
webhook) plug into `internal/eventsink` without touching engine internals.
There is no shared mutable state beyond Cobra flag bindings.

### Carry context modes

| Mode | Behaviour |
| ---- | --------- |
| `off` | Each iteration only sees the original prompt. |
| `summary` | The assistant's last response is summarised and prepended (default). |
| `verbatim` | The raw last response is prepended (up to `--carry-context-max-runes`). |

## Exit codes

| Code | Meaning |
| ---- | ------- |
| `0`  | Loop finished cleanly (promise received or no-op). |
| `1`  | Generic failure / SDK error. |
| `2`  | Cancelled (`Ctrl+C` or invalid args). |
| `3`  | `--timeout` exceeded. |
| `4`  | `--max-iterations` reached without a promise. |

## Development

```bash
make all        # tidy + fmt + vet + lint + test + build
make test       # go test -race -cover ./...
make build      # ./bin/ralph
```

Conventions, architecture, and "how to add X" live in [`.github/copilot-instructions.md`](.github/copilot-instructions.md).
Contribution workflow is in [CONTRIBUTING.md](./CONTRIBUTING.md). User-visible
changes belong in [CHANGELOG.md](./CHANGELOG.md). Security disclosures go
through [SECURITY.md](./SECURITY.md).

## Acknowledgements

- The original [Ralph Wiggum plugin](https://github.com/anthropics/claude-code/tree/main/plugins/ralph-wiggum)
  for Claude Code that inspired the loop pattern.
- The [GitHub Copilot SDK](https://github.com/github/copilot-sdk) team.

## License

[MIT](./LICENSE)

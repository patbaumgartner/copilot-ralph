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
  `<promise>...</promise>`; Ralph emits an event when it sees one.
- **Hard limits.** `--max-iterations` and `--timeout` keep runaway loops
  in check.
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

## Usage

```bash
# Inline prompt
ralph run "Add unit tests for the parser module"

# Markdown file as prompt
ralph run task.md

# Cap iterations and runtime
ralph run --max-iterations 5 --timeout 10m "Refactor authentication"

# Show the resolved config without calling the model
ralph run --dry-run "Implement OAuth"

# Custom system prompt (note: --system-prompt-mode=replace removes Ralph's
# built-in <promise>...</promise> instruction)
ralph run \
  --system-prompt prompts/expert-go.md \
  --system-prompt-mode replace \
  "Optimise the hot path"
```

### Useful flags

`ralph --help` lists every flag. The most common ones:

| Flag                              | Default        | Purpose                                          |
| --------------------------------- | -------------- | ------------------------------------------------ |
| `-m`, `--max-iterations`          | `10`           | Stop after N loops.                              |
| `-t`, `--timeout`                 | `30m`          | Stop after this duration.                        |
| `--iteration-timeout`             | `0`            | Per-iteration soft timeout (0 disables).         |
| `--promise`                       | `I'm special!` | Phrase the model wraps in `<promise>`.           |
| `--model`                         | `gpt-4`        | Copilot model id.                                |
| `--working-dir`                   | cwd            | Where the assistant runs tools.                  |
| `--log-level`                     | `info`         | `debug` / `info` / `warn` / `error`.             |
| `--streaming`                     | `true`         | Stream deltas vs. wait for full messages.        |
| `--system-prompt`                 | (built-in)     | Inline text or path to a Markdown file.          |
| `--system-prompt-mode`            | `append`       | `append` or `replace` Ralph's system prompt.     |
| `--carry-context`                 | `summary`      | `off` / `summary` / `verbatim`.                  |
| `--carry-context-max-runes`       | `4000`         | Max runes carried into the next prompt.          |
| `--prompt-stack`                  | (none)         | Extra prompts/files prepended each iteration.    |
| `--plan-file`                     | (none)         | Path to a running fix plan Markdown file.        |
| `--specs`                         | (none)         | Directory whose Markdown specs are listed.       |
| `--stop-on-no-changes`            | `0`            | Halt after N iterations without changes.         |
| `--stop-on-error`                 | `0`            | Halt after N iterations emitting errors.         |
| `--verify-cmd`                    | (none)         | Shell command run after each iteration.          |
| `--verify-timeout`                | `5m`           | Timeout for a single `--verify-cmd` run.         |
| `--verify-max-bytes`              | `16384`        | Max bytes captured per `--verify-cmd` stream.    |
| `--auto-commit`                   | `false`        | `git add -A && commit` after each iteration.     |
| `--auto-commit-message`           | `ralph: iteration %d` | Format string for auto-commit messages.   |
| `--auto-commit-on-failure`        | `false`        | Auto-commit even when `--verify-cmd` failed.     |
| `--auto-tag`                      | (none)         | Annotated tag format for auto-commits.           |
| `--diff-stat`                     | `false`        | Emit `git diff --stat HEAD` each iteration.      |
| `--json`                          | `false`        | Emit JSON Lines envelopes to stdout.             |
| `--json-output`                   | (none)         | Also write JSON Lines to this file.              |
| `--log-file`                      | (none)         | Append a one-line summary of every event.        |
| `--webhook`                       | (none)         | POST every event as JSON to this URL.            |
| `--webhook-timeout`               | `5s`           | Timeout for a single webhook delivery.           |
| `--checkpoint-file`               | (none)         | Persist loop state for `ralph resume`.           |
| `--oracle-model`                  | (none)         | Second-opinion model consulted between iters.    |
| `--oracle-every`                  | `0`            | Consult the oracle every N iterations.           |
| `--oracle-on-verify-fail`         | `false`        | Consult the oracle whenever verify fails.        |
| `--no-rate-limit-wait`            | `false`        | Fail immediately on rate limit errors.           |
| `--dry-run`                       | `false`        | Print the config and exit.                       |

### Commands

| Command         | Purpose                                                      |
| --------------- | ------------------------------------------------------------ |
| `ralph run`     | Run the iteration loop with the given prompt.                |
| `ralph resume`  | Resume from a `--checkpoint-file` written by a prior run.    |
| `ralph reset`   | Delete a checkpoint file (`--force` to skip the prompt).     |
| `ralph doctor`  | Run environment health checks (Copilot CLI, git, etc.).      |
| `ralph version` | Print build metadata.                                        |

## How it works

```text
   prompt ──► Copilot SDK ──► assistant tokens, tool calls
                  ▲                      │
                  └──── next iteration ──┘
                  (until promise / max-iterations / timeout / Ctrl+C)
```

The loop lives in `internal/core`. The SDK wrapper (`internal/sdk`) handles
sessions, retries, and translates SDK events into Ralph-flavoured events.
The CLI layer (`cmd/ralph`, `internal/cli`) wires flags to a `LoopConfig`
and prints events as they stream in. There is no shared mutable state
beyond Cobra flag bindings.

## Exit codes

| Code | Meaning                                              |
| ---- | ---------------------------------------------------- |
| `0`  | Loop finished cleanly.                               |
| `1`  | Generic failure / SDK error.                         |
| `2`  | Cancelled (`Ctrl+C` or invalid args).                |
| `3`  | `--timeout` exceeded.                                |
| `4`  | `--max-iterations` reached without a promise.        |

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
- [Charmbracelet](https://github.com/charmbracelet) for `lipgloss`.

## License

[MIT](./LICENSE)

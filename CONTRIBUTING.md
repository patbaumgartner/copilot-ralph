# Contributing to Ralph

Thanks for your interest in improving Ralph! This document covers the practical
bits you need to ship a clean pull request.

## Development setup

You need:

- Go 1.26 or newer (matches `go.mod`).
- The [GitHub Copilot CLI](https://github.com/github/copilot) on your `$PATH`
  if you want to exercise the SDK integration tests locally.
- `golangci-lint` **built with the same Go toolchain you use to develop**.
  If `golangci-lint run` fails with a "Go language version is lower than
  targeted" error, reinstall with `make dev-deps` using Go 1.26+.

```bash
git clone https://github.com/patbaumgartner/copilot-ralph.git
cd copilot-ralph
go mod download
make build
```

## Quality gates

Before opening a pull request, run:

```bash
make tidy   # go mod tidy + verify
make fmt    # gofmt -w -s
make vet    # go vet ./...
make lint   # golangci-lint
make test   # go test -race -cover ./...
make build  # cross-checks the CLI builds
```

`make all` chains the same steps. CI runs the equivalent on every push and
pull request.

## Conventions

- **No `else` branches** — use early returns. (Enforced by review, not lint.)
- **Always wrap errors with `%w`** when adding context.
- **Table-driven tests** for any function with more than one interesting
  input.
- **Document exported symbols.** A short `// Foo does X.` is enough.
- **Keep packages focused.** `internal/cli` orchestrates, `internal/core`
  owns business logic, `internal/sdk` is the only place that talks to the
  Copilot SDK, `internal/tui/styles` owns terminal rendering helpers.
- **Update the `[Unreleased]` section of [CHANGELOG.md](./CHANGELOG.md)** for
  any user-visible change.

See [`.github/copilot-instructions.md`](.github/copilot-instructions.md) for the longer architectural tour.

## Testing

- New functions need at least one happy-path and one edge-case test.
- Bug fixes need a regression test.
- Tests that require the Copilot CLI must call `skipIfNoSDK(t)` so they are
  skipped automatically in CI.

## Pull requests

- One topic per PR. Smaller is better.
- Include a short rationale and link any related issue.
- Make sure CI is green before requesting review.

## Reporting bugs

Open a [GitHub issue](https://github.com/patbaumgartner/copilot-ralph/issues)
with:

- A minimal reproduction (command line + expected vs. actual output).
- Your `ralph version` output.
- The Copilot SDK / CLI version if relevant.

## Code of conduct

This project follows the [Contributor Covenant Code of Conduct](./CODE_OF_CONDUCT.md).
By participating you agree to abide by its terms.

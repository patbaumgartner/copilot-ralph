# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-04-27

### Changed

- Renamed module path to `github.com/patbaumgartner/copilot-ralph`.
- `LoopConfig` now carries SDK-facing settings (`Streaming`, `LogLevel`,
  `SystemPrompt`, `SystemPromptMode`) so the CLI no longer reaches into
  package-level globals from `createSDKClient`.
- `ralph run` returns a typed `ExitError` instead of calling `os.Exit` from
  inside `RunE`, allowing deferred SDK shutdown to run.
- `ralph run` now requires exactly one positional argument (`cobra.ExactArgs(1)`)
  and no longer panics when invoked without a prompt.
- Documentation realigned with the actual CLI-only architecture (no Bubble Tea
  TUI, no `ralph init`, no `/specs` directory).

### Removed

- Stale `docker-build`, `docker-run`, and `release-snapshot` Makefile targets
  whose configs were never present.
- Committed `coverage_sdk` profile artifact.
- Dead `fakeSession` / `testSessionAdapter` helpers in the SDK test suite.

### Fixed

- SDK tests now compile against `github.com/github/copilot-sdk/go v0.3.0`
  (typed `SessionEventData` implementations rather than the pre-release
  `copilot.Data` flat struct).
- Documentation comment on `detectPromise` now matches its implementation
  (exact `<promise>...</promise>` substring match).
- `ralph run` warns when `--system-prompt-mode=replace` removes Ralph's
  built-in promise instructions.
- Initial iteration of the Ralph loop driver.

[Unreleased]: https://github.com/patbaumgartner/copilot-ralph/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/patbaumgartner/copilot-ralph/releases/tag/v0.1.0

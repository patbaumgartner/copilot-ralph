# Security Policy

## Supported Versions

Ralph is pre-1.0 and only the `main` branch is supported. Security fixes will
land on `main` and ship in the next tagged release.

## Reporting a Vulnerability

Please **do not** file public GitHub issues for security problems.

Instead, use GitHub's private vulnerability reporting:

1. Go to <https://github.com/patbaumgartner/copilot-ralph/security/advisories/new>.
2. Describe the issue, impact, and a reproduction if possible.

You can expect:

- An acknowledgement within **5 business days**.
- A triage decision (accept / decline / need more info) within **14 days**.
- Coordinated disclosure once a fix is available.

## Scope

In scope:

- The `ralph` CLI binary and its packages under `cmd/` and `internal/`.
- Build/release tooling shipped from this repository.

Out of scope:

- Vulnerabilities in upstream dependencies — please report those upstream.
- Issues that require an attacker to already have local code execution as
  the user running Ralph.

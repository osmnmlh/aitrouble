# Contributing to aitrouble

Thank you for your interest in contributing! This document outlines the process for contributing to `aitrouble`.

## Before You Start

- Check [open issues](https://github.com/osmnmlh/aitrouble/issues) for existing discussions.
- For significant changes, open an issue first to discuss the approach.
- Look for [`good first issue`](https://github.com/osmnmlh/aitrouble/labels/good%20first%20issue) labels if you're new.

## Development Setup

```bash
git clone https://github.com/osmnmlh/aitrouble.git
cd aitrouble
go mod download
go test -v -race ./...
```

## Core Constraints (Non-negotiable)

| Constraint | Rule |
|---|---|
| **Zero secrets in output** | API keys must always be redacted. Use `internal/redact` package. |
| **Read-only** | Never write to user files, env, or filesystem. |
| **Zero telemetry** | No analytics, beaconing, or crash reporting. |
| **No new deps without discussion** | Open an issue first for any `go.mod` addition. |
| **Race-free** | All code must pass `go test -race`. |

## Pull Request Process

1. Fork the repository and create a branch: `git checkout -b type/short-description`
2. Write tests for your changes.
3. Ensure `go test -v -race ./...` and `go vet ./...` pass.
4. Submit a PR using the [PR template](.github/pull_request_template.md).
5. Link the PR to the relevant issue (`Closes #N`).

## Branch Naming

| Type | Pattern | Example |
|---|---|---|
| Spike | `spike/m0-description` | `spike/m0-effective-config` |
| Feature | `feat/short-description` | `feat/tcp-probe` |
| Bug fix | `fix/short-description` | `fix/tls-timeout` |
| Docs | `docs/short-description` | `docs/readme-quickstart` |

## Commit Style

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(config): add .env precedence over shell env
fix(network): handle TCP timeout on Windows
docs(readme): add Azure OpenAI example
test(provider): add fixture for 401 auth failure
```

## Code Style

- Run `gofmt -w .` before committing.
- Prefer `errors.New` / `fmt.Errorf` over `panic`.
- Keep functions small and testable.
- Document all exported symbols.

## Security Reporting

Do **not** open a public issue for security vulnerabilities. Email `security@aitrouble.dev` (or use GitHub's private vulnerability reporting).

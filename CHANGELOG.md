# Changelog

All notable changes to `aitrouble` will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-09-28

### Added

- **`aitrouble doctor`** — single command that orchestrates the full diagnostic chain and prints a deterministic, human-readable result
- **Effective Configuration** — resolves shell environment → `.env` file → default precedence; supports `--env-file` flag
- **Network probes** — DNS resolution, TCP connectivity, TLS handshake with short-circuit on first failure
- **Provider probe** — OpenAI-compatible `GET /models` endpoint check; classifies HTTP 401/403/404/429/5xx responses distinctly
- **Deterministic diagnosis** — maps each `FailureKind` to a `FailingLayer`, human-readable `Summary`, and concrete `FixHint`
- **Local MCP configuration discovery** — static (read-only) discovery of MCP server definitions from Cursor, Claude Desktop, VS Code workspace, and portable `.mcp.json`; never executes discovered commands
- **Exit-code semantics** — `0` healthy, `1` failure detected, `2` CLI usage error

### Security

- API keys and secrets are never printed; output shows `[REDACTED]`
- MCP HTTP URL credentials and sensitive query parameters are redacted before display
- MCP environment variable values are never stored or printed (count only)
- No MCP server commands are executed (M5A is static discovery only)
- Zero telemetry: no data leaves the machine
- Read-only: no config files are modified

[Unreleased]: https://github.com/osmnmlh/aitrouble/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/osmnmlh/aitrouble/compare/4b825dc642cb6eb9a060e54bf8d69288fbee4904...v0.1.0

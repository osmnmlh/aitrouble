# aitrouble

<div align="center">

**Find where your AI integration breaks.**

[![CI](https://github.com/osmnmlh/aitrouble/actions/workflows/ci.yml/badge.svg)](https://github.com/osmnmlh/aitrouble/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.27-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/osmnmlh/aitrouble?color=blueviolet)](https://github.com/osmnmlh/aitrouble/releases)

</div>

---

## The problem

AI integrations break in ways that are hard to isolate:

- **Wrong effective configuration** — your `.env` sets `OPENAI_BASE_URL` but the shell environment overrides it silently with a stale value
- **Unreachable endpoint** — a local proxy or custom gateway is down, DNS fails, or a firewall blocks the port
- **Provider rejection** — authentication fails (401), a deployment path is wrong (404), or the account is rate-limited (429)

`aitrouble doctor` runs a deterministic, layered probe sequence and tells you exactly which layer is broken and why — without guessing.

---

## Demo

```
$ aitrouble doctor
aitrouble doctor
Find where your AI integration breaks.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[1/4] Effective Configuration
  ✓  OPENAI_API_KEY     [REDACTED]                source: shell
  ✓  OPENAI_BASE_URL    https://api.openai.com/v1  source: .env

[2/4] Network Probes
  ✓  DNS api.openai.com                           (12ms)
  ✓  TCP api.openai.com:443                       (23ms)
  ✓  TLS api.openai.com:443                       (41ms)

[3/4] Provider Probe
  ✗  OpenAI /models                               [auth_failure]
     Evidence: HTTP 401

[4/4] Local MCP
  ✓  Cursor
     2 server(s) configured

     filesystem
       transport: stdio
       command: npx
       args: 2

     github
       transport: stdio
       command: docker
       args: 3

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 DIAGNOSIS  The chain breaks at: Provider › Authentication
 SUMMARY    The network path succeeded, but the provider rejected authentication.
 FIX        Check the active OPENAI_API_KEY in your shell or .env file.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

## Core Principles

| Principle | What it means |
|---|---|
| 🏠 **Local-first** | Runs entirely on your machine. No remote agents, no SaaS, no accounts. |
| 🔕 **Zero-telemetry** | No analytics, no crash reporting, no beaconing of any kind. |
| 🔒 **Secret-safe** | API keys, tokens, and MCP URL credentials are redacted from all output. |
| 📦 **Single-binary** | One static binary. `go install` and you're done. No runtime dependencies. |
| 📖 **Read-only** | `aitrouble` never writes to your config files or executes discovered MCP commands. |

---

## Install

### Via `go install`

```bash
go install github.com/osmnmlh/aitrouble/cmd/aitrouble@latest
```

### Pre-built binaries

Download the binary for your platform from the [Releases page](https://github.com/osmnmlh/aitrouble/releases).

| Platform | Archive |
|---|---|
| Linux (x86\_64) | `aitrouble_Linux_amd64.tar.gz` |
| macOS (Intel) | `aitrouble_Darwin_amd64.tar.gz` |
| macOS (Apple Silicon) | `aitrouble_Darwin_arm64.tar.gz` |
| Windows (x86\_64) | `aitrouble_Windows_amd64.zip` |

Checksums are provided as `checksums.txt` (SHA-256).

---

## Usage

```bash
# Print version
aitrouble --version

# Run full diagnostic in current directory
aitrouble doctor

# Use a specific .env file
aitrouble doctor --env-file /path/to/.env
```

**Exit codes:**

| Code | Meaning |
|---|---|
| `0` | All tested components appear healthy |
| `1` | A failure was detected in at least one layer |
| `2` | CLI usage error |

---

## How It Works

```
      Your Project Directory
               │
               ▼
┌───────────────────────────────┐
│  1. Config Engine             │  shell env → .env → default
│     → Effective Configuration │  Secrets are never printed
└──────────────┬────────────────┘
               │
               ▼
┌───────────────────────────────┐
│  2. Network Probes            │  DNS → TCP → TLS
│     → Pass / Fail + Evidence  │  Short-circuit on first failure
└──────────────┬────────────────┘
               │
               ▼
┌───────────────────────────────┐
│  3. Provider Probe            │  OpenAI-compatible GET /models
│     → Pass / Fail + Evidence  │  Skipped if network already failed
└──────────────┬────────────────┘
               │
               ▼
┌───────────────────────────────┐
│  4. MCP Discovery (static)    │  Cursor · Claude Desktop · VS Code
│     → Config inventory        │  Never executes commands
└──────────────┬────────────────┘
               │
               ▼
        DETERMINISTIC DIAGNOSIS + FIX HINT
```

> **Planned (M5B):** Local MCP process health probing will extend the chain with an active runtime check.

---

## Current Capabilities

| Capability | Status |
|---|---|
| Effective Configuration (`shell → .env → default`) | ✅ |
| DNS probe | ✅ |
| TCP probe | ✅ |
| TLS probe | ✅ |
| OpenAI-compatible `/models` probe | ✅ |
| `aitrouble doctor` orchestration | ✅ |
| Deterministic failure diagnosis | ✅ |
| Local MCP config discovery (static) | ✅ |
| Exit-code semantics | ✅ |
| MCP process health probing (M5B) | ⏳ Planned |
| JSON output | ⏳ Planned |
| TUI | ⏳ Planned |
| Additional provider profiles | ⏳ Planned |

### MCP Discovery Sources

`aitrouble doctor` discovers MCP server definitions from the following locations (read-only, no process execution):

| Source | Location |
|---|---|
| Cursor | `~/.cursor/mcp.json` |
| Claude Desktop (Windows) | `%APPDATA%\Claude\claude_desktop_config.json` |
| Claude Desktop (macOS) | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Claude Desktop (Linux) | `$XDG_CONFIG_HOME/Claude/claude_desktop_config.json` |
| VS Code workspace | `<cwd>/.vscode/mcp.json` |
| Portable | `<cwd>/.mcp.json` |

---

## Documentation

- [Architecture](docs/architecture.md)
- [Use Cases](docs/use-cases.md)
- [Requirements](docs/requirements.md)

---

## Contributing

We welcome contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

- **Bug reports:** Use the [bug report template](.github/ISSUE_TEMPLATE/bug_report.yml)
- **Feature requests:** Use the [feature request template](.github/ISSUE_TEMPLATE/feature_request.yml)
- **Good first issues:** Look for the [`good first issue`](https://github.com/osmnmlh/aitrouble/labels/good%20first%20issue) label

### Development Setup

```bash
git clone https://github.com/osmnmlh/aitrouble.git
cd aitrouble
go mod download
go test -race ./...
go build ./cmd/aitrouble
```

### Testing

For end-to-end release validation and QA, we use a fully automated suite. See [organic-tests/README.md](organic-tests/README.md) for instructions on running the organic-tests test harness.

---

## Roadmap

| Milestone | Status |
|---|---|
| **M1** – Core Engine & Effective Config | ✅ Complete |
| **M2** – Network Probes | ✅ Complete |
| **M3** – Provider Probe | ✅ Complete |
| **M4** – Doctor + Deterministic Diagnosis | ✅ Complete |
| **M5A** – Local MCP Discovery | ✅ Complete |
| **M5B** – Local MCP Process Probe | ⏳ Planned |

---

## License

MIT © 2026 [osmnmlh](https://github.com/osmnmlh)

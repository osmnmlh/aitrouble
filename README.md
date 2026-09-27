# aitrouble

<div align="center">

**Find where your AI integration breaks.**

[![CI](https://github.com/osmnmlh/aitrouble/actions/workflows/ci.yml/badge.svg)](https://github.com/osmnmlh/aitrouble/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.27-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/osmnmlh/aitrouble)](https://goreportcard.com/report/github.com/osmnmlh/aitrouble)
[![Release](https://img.shields.io/github/v/release/osmnmlh/aitrouble?color=blueviolet)](https://github.com/osmnmlh/aitrouble/releases)

</div>

---

## Why is your AI integration down? Stop guessing.

`aitrouble` is a zero-dependency, read-only CLI. Run `aitrouble doctor` to get a deterministic, human-readable diagnosis of exactly where your AI integration fails — no guessing, no secrets leaked.

**Implemented today:** Effective Configuration · DNS · TCP · TLS · OpenAI-compatible `/models` · `aitrouble doctor` · deterministic diagnosis · local MCP config discovery · exit-code semantics

**Planned:** MCP process probing (M5B) · JSON output · TUI · LLM diagnosis · additional provider profiles

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
| 🔒 **Secret-safe** | Secrets are redacted from aitrouble output. Authenticated requests, when enabled, are sent only to the configured provider endpoint. |
| 📦 **Single-binary** | One static binary. `go install` and you're done. No runtime dependencies. |
| 📖 **Read-only** | `aitrouble` never writes to your config files. It only reads and probes. |

---

## Quick Start

### Install via `go install`

```bash
go install github.com/osmnmlh/aitrouble/cmd/aitrouble@latest
```



### Run

```bash
# Check version
aitrouble --version

# Run full diagnostic in current directory
aitrouble doctor

# Target a specific .env file
aitrouble doctor --env-file /path/to/.env
```

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
│  2. Network Probes            │  DNS → TCP → TLS (HTTPS)
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
        DETERMINISTIC DIAGNOSIS + FIX HINT
```

> **Planned (M5):** Local MCP process probing will extend the chain above.

---

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
go test ./...
go build ./cmd/aitrouble
```

---

## Roadmap

| Milestone | Target | Status |
|---|---|---|
| **M0** – Spike Verification | Week 1 | ✅ Complete |
| **M1** – Core Engine & Effective Config | Week 2 | ✅ Complete |
| **M2** – Network Probes | Week 3 | ✅ Complete |
| **M3** – Provider Probe | Week 4 | ✅ Complete |
| **M4** – Doctor + Deterministic Diagnosis | Week 5 | ✅ Complete |
| **M5A** – Local MCP Discovery | Week 6 | ✅ Complete |
| **M5B** – Local MCP Process Probe | Week 7 | ⏳ Planned |

---

## License

MIT © 2026 [osmnmlh](https://github.com/osmnmlh)

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

`aitrouble` is a zero-dependency, read-only CLI that reads your project's config layers, builds the **Effective Configuration** hierarchy, then probes every link in the chain—DNS → TCP → TLS → HTTP → Provider API → MCP—and tells you **exactly** which link is broken, with deterministic evidence.

---

## Demo (Planned output)

```
$ aitrouble doctor

 aitrouble v0.1.0  •  Find where your AI integration breaks
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[1/4] Effective Configuration
  ✓  Source: shell env   OPENAI_API_KEY  = sk-•••••••••••••••••••oXyZ
  ✓  Source: .env file   OPENAI_BASE_URL = https://api.openai.com/v1
  ⚠  Source: .env file   OPENAI_TIMEOUT  = (not set, using default 30s)

[2/4] Network Probes
  ✓  DNS   api.openai.com → 104.18.7.192  (12ms)
  ✓  TCP   104.18.7.192:443               (23ms)
  ✓  TLS   CN=openai.com  TLS 1.3         (41ms)
  ✗  HTTP  POST /v1/chat/completions      → 401 Unauthorized

[3/4] Provider Probe
  ✗  OpenAI /v1/models  → 401 Unauthorized
     Evidence: {"error":{"code":"invalid_api_key","type":"..."}}

[4/4] Local MCP
  –  No MCP server config detected (skipped)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 DIAGNOSIS  The chain breaks at: Provider › invalid_api_key
 FIX        Regenerate your API key at https://platform.openai.com/api-keys
            and update OPENAI_API_KEY in your shell or .env file.
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
# Check version (currently implemented)
aitrouble --version

# Run full diagnostic in current directory (Planned)
aitrouble doctor

# Target a specific .env file (Planned)
aitrouble doctor --env-file /path/to/.env
```

---

## How It Works (Planned Architecture)

```
Your Project Directory
        │
        ▼
┌───────────────────────────────┐
│  1. Config Engine             │  Reads shell env, .env, project config
│     → Effective Configuration │  Builds precedence-aware merged config
└──────────────┬────────────────┘
               │
               ▼
┌───────────────────────────────┐
│  2. Network Probes            │  DNS → TCP → TLS → HTTP
│     → Pass / Fail + Evidence  │  Deterministic, timeout-bounded
└──────────────┬────────────────┘
               │
               ▼
┌───────────────────────────────┐
│  3. Provider Probe            │  OpenAI / OpenAI-compatible /v1/models
│     → Pass / Fail + Evidence  │  Auth check without sensitive logging
└──────────────┬────────────────┘
               │
               ▼
┌───────────────────────────────┐
│  4. MCP Process Probe         │  Local MCP server health check
│     → Pass / Skip + Evidence  │  Detects stdio / HTTP MCP servers
└──────────────┬────────────────┘
               │
               ▼
        DIAGNOSIS + FIX HINT
```

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
| **M2** – Network Probes | Week 3 | 🔄 In Progress |
| **M3** – Local MCP & Doctor CLI v0.1 | Week 4 | ⏳ Planned |

---

## License

MIT © 2026 [osmnmlh](https://github.com/osmnmlh)

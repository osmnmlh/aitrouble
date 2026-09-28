# Use Cases

## 1. Overview

`aitrouble` quickly identifies where an AI integration breaks by probing the effective configuration, network path, provider API, and local MCP configuration — and pinpointing the first failing layer.

## 2. Actors

- Developer
- AI application developer
- Local LLM developer
- DevOps / platform engineer
- MCP / agent developer
- CI/CD user

## 3. Current Supported Use Cases

The `aitrouble doctor` command executes a full diagnostic chain and presents a deterministic diagnosis.

### UC-01 — Effective configuration resolution

Resolves the active `OPENAI_API_KEY` and `OPENAI_BASE_URL` using precedence:
`shell env` → `.env` → `default`. Surfaces surprising overrides before any network probe runs.

### UC-02 — DNS failure

If DNS fails to resolve the hostname, downstream TCP and TLS probes are short-circuited. DNS is explicitly identified as the fault.

### UC-03 — TCP refusal

DNS succeeds but the destination actively refuses the TCP connection.

### UC-04 — TCP timeout

DNS succeeds but the TCP handshake times out (firewall, wrong port, service not running).

### UC-05 — TLS failure

DNS and TCP succeed, but the TLS handshake fails (certificate error, expired cert, SNI mismatch).

### UC-06 — Provider authentication failure

Network passes (DNS ✓, TCP ✓, TLS ✓), but the provider responds with `401` or `403`. The API key is wrong or inactive.

### UC-07 — Provider route not found

The OpenAI-compatible `/models` route returns `404`. The base URL path is incorrect (e.g., missing `/v1`).

### UC-08 — Provider rate limit

`/models` returns `429`. The account is rate-limited or quota is exhausted.

### UC-09 — Provider server error

The provider responds with `5xx`. The service is degraded.

### UC-10 — Local or HTTP endpoint

When `OPENAI_BASE_URL` uses `http://` (e.g., a local proxy on `localhost:11434`), TLS is bypassed automatically and only DNS + TCP are probed.

### UC-11 — MCP configuration discovery

When `aitrouble doctor` runs, it discovers MCP server definitions from well-known client configuration files (Cursor, Claude Desktop, VS Code workspace, portable `.mcp.json`). The output shows:

- which MCP configuration sources were found
- how many server definitions each contains
- transport type (stdio / http) and command or redacted URL

This is a **static, read-only** check. No MCP servers are started or queried.

### UC-12 — Malformed MCP configuration

If a discovered MCP configuration file exists but cannot be parsed (invalid JSON, wrong structure), `aitrouble doctor` reports it as a `MCP › Configuration` failure and exits with code `1`.

## 4. Planned Use Cases

- **UC-P1 — MCP process health probe (M5B):** Start and verify that discovered MCP servers are actually running and responsive via the MCP initialize handshake.
- **UC-P2 — JSON output:** Machine-readable `--json` flag for CI/CD integration.
- **UC-P3 — Additional provider profiles:** Azure OpenAI, Anthropic, Google Gemini.

## 5. Diagnostic Chain

```text
1. Load configuration (EffectiveConfig)
2. Test network health (DNS → TCP → TLS)
3. Test provider authentication (GET /models)
4. Discover local MCP configuration (static)
5. Correlate results → deterministic Diagnosis → exit code
```

Network and provider failures take priority over MCP configuration issues in the diagnosis output.

## 6. Example Failure Scenarios

### Scenario A — Wrong API key

```text
DNS ✓  TCP ✓  TLS ✓  Provider ✗ 401
```

**Diagnosis:** `Provider › Authentication` — network path succeeded; provider rejected authentication.

### Scenario B — DNS failure

```text
DNS ✗  TCP –  TLS –  Provider –
```

**Diagnosis:** `Network › DNS` — hostname could not be resolved; downstream probes skipped.

### Scenario C — Wrong provider route

```text
DNS ✓  TCP ✓  TLS ✓  Provider ✗ 404
```

**Diagnosis:** `Provider › /models` — target is reachable, but the `/models` route was not found. Check `OPENAI_BASE_URL` path.

### Scenario D — Malformed MCP config (all network probes pass)

```text
DNS ✓  TCP ✓  TLS ✓  Provider ✓  MCP ✗ (malformed JSON)
```

**Diagnosis:** `MCP › Configuration` — all provider checks passed, but a discovered MCP config file is invalid.

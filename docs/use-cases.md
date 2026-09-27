# Use Cases

## 1. Overview

`aitrouble` exists to quickly identify where an AI integration breaks by systematically probing the effective configuration, network topology, and provider requirements.

## 2. Actors

The intended users of the tool include:
- Developer
- AI application developer
- Local LLM developer
- DevOps / platform engineer
- MCP / agent developer
- CI/CD user

## 3. Current Supported Use Cases

The current system supports an end-to-end CLI `doctor` experience that executes these component-level checks and aggregates the results into a final diagnosis.

### UC-01 — Effective configuration resolution
Safely reading the API key and Base URL to determine whether to use:
`shell env` vs `.env` vs `default`.

### UC-02 — DNS failure
If DNS fails to resolve a hostname, the system short-circuits. Downstream TCP and TLS probes are safely aborted, explicitly highlighting DNS as the fault.

### UC-03 — TCP refusal
If DNS succeeds but the destination actively refuses a TCP connection.

### UC-04 — TCP timeout
If DNS succeeds but the TCP handshake times out.

### UC-05 — TLS failure
If DNS and TCP succeed, but the TLS handshake fails (e.g., certificate validation errors).

### UC-06 — Provider authentication failure
If the network stack passes (DNS ✓, TCP ✓, TLS ✓), but the provider responds with `401` or `403`.

### UC-07 — Provider route not found
If the expected OpenAI-compatible `/models` route returns a `404`.

### UC-08 — Provider rate limit
If the expected `/models` route returns a `429`.

### UC-09 — Provider server error
If the provider responds with `5xx`.

### UC-10 — Local OpenAI-compatible endpoint
If the developer specifies a local target (e.g., `http://localhost:...`), `aitrouble` handles HTTP (bypassing TLS) seamlessly to probe the endpoint.

## 4. Planned Use Cases

*These use cases are planned but not yet implemented:*
- local MCP process health checks
- MCP configuration discovery
- JSON output
- CI/CD integration
- additional provider profiles

## 5. Diagnostic Chain

When a full diagnostic workflow runs, it follows a rigorous hierarchy of trust:
1. Load configuration (`EffectiveConfig`).
2. Test network health (`NetworkProber`).
3. Test provider authentication/API shapes (`ProviderProber`).
4. (Planned) Aggregate faults into `core.Diagnosis`.

## 6. Example Failure Scenarios

### Scenario A — Wrong API key
```text
DNS ✓
TCP ✓
TLS ✓
Provider ✗ 401
```
**Interpretation:** Network path works. Provider rejected authentication.

### Scenario B — DNS problem
```text
DNS ✗
TCP –
TLS –
Provider –
```
**Interpretation:** The failure occurred before TCP/provider access.

### Scenario C — Wrong provider route
```text
DNS ✓
TCP ✓
TLS ✓
Provider ✗ 404
```
**Interpretation:** The target is reachable, but `/models` was not found. (Note: A 404 implies the route is missing, but does not definitively prove global incompatibility.)

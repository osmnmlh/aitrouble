## Spike Goal

Prove that `aitrouble` can safely probe an OpenAI-compatible `/v1/models` endpoint — obtaining meaningful evidence about auth, connectivity, and compatibility — **without leaking the API key** in any output, log line, or error message.

This spike lives in `spike/m0-provider-probe/main.go`.

---

## Context

The provider probe is the most sensitive part of `aitrouble`. It must:
1. Make an authenticated HTTP request (Bearer token in `Authorization` header)
2. Parse the response to determine if auth succeeded, failed, or if the endpoint is incompatible
3. **Never** include the raw API key in any output, even in debug/verbose mode

The probe should work against any OpenAI-compatible API (OpenAI, LM Studio, Ollama with OpenAI adapter, Azure OpenAI with the appropriate base URL, etc.).

---

## Acceptance Criteria

- [ ] `ProviderProbe(baseURL, apiKey string, timeout time.Duration) ProbeResult` implemented  
- [ ] Uses only `net/http` from stdlib — no third-party HTTP or OpenAI SDK  
- [ ] Probes `GET {baseURL}/v1/models` with `Authorization: Bearer {apiKey}`  
- [ ] **Auth success (200)**: Evidence includes model count and first 3 model IDs  
- [ ] **Auth failure (401/403)**: Evidence includes HTTP status + provider error JSON (without echoing the key)  
- [ ] **Wrong path / 404**: Evidence identifies endpoint as "non-OpenAI-compatible"  
- [ ] **Network error**: Evidence from underlying TCP/TLS error (no key in error string)  
- [ ] Secret redaction: API key is **never** present in any `ProbeResult` field, even if the server echoes it back in an error body  
- [ ] Unit tests use `httptest.NewServer` to simulate: 200 with model list, 401 with error JSON, 404, timeout  
- [ ] Redaction is verified by a unit test: inject a fake key → assert key string absent from all output fields  
- [ ] Spike binary accepts `--base-url` and `--api-key` flags, prints result table with redacted key shown as `sk-•••oXyZ`  

---

## Security Notes

> This is the single most security-critical piece of code in the spike phase.

- The `Authorization` header must never appear in `Evidence`, `error.Error()`, or any formatted output
- Response bodies must be scanned for the raw key value and redacted before storing in `Evidence`
- Use a `transport.RoundTripper` wrapper that strips auth headers from error messages

---

## Out of Scope

- TLS certificate validation details — M2
- Azure-specific deployment probes — M2+

---

## Definition of Done

PR merged from `spike/m0-provider-probe` with all AC checked, secret redaction test passing, CI green.

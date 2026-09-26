## Spike Goal

Prove that `aitrouble` can emit **deterministic** evidence for two distinct network failure modes:

1. **Connection Refused** — port is closed, RST received immediately
2. **Timeout** — no response within deadline (e.g., firewall drop, wrong IP)

This spike lives in `spike/m0-tcp-probe/main.go`.

---

## Context

The most common reason AI integrations fail silently is a misconfigured `OPENAI_BASE_URL` pointing to a host that either refuses connections or simply times out. The diagnostic must distinguish these two cases — they have different root causes and different fixes.

The probe must be:
- **Deterministic**: same failure mode → same evidence string, every run
- **Timeout-bounded**: never hangs longer than a configurable deadline
- **OS-agnostic**: works on Linux, macOS, Windows

---

## Acceptance Criteria

- [ ] `TCPProbe(host string, port int, timeout time.Duration) ProbeResult` function implemented in `spike/m0-tcp-probe/`  
- [ ] `ProbeResult` contains: `Status` (pass/fail), `Latency time.Duration`, `Evidence string`, `FailureKind` (refused | timeout | dns_error | pass)  
- [ ] **Connection refused**: `FailureKind = "refused"`, `Evidence` contains OS error string (normalized across platforms)  
- [ ] **Timeout**: `FailureKind = "timeout"`, `Evidence` contains `"deadline exceeded after Xms"`  
- [ ] **DNS failure**: `FailureKind = "dns_error"`, `Evidence` contains the unresolved hostname  
- [ ] **Pass**: `FailureKind = "pass"`, `Latency` is the measured TCP handshake time  
- [ ] Unit tests use `net.Listen` on a random port to simulate "refused" (close listener before connecting) and a custom `DialContext` mock to simulate timeout  
- [ ] Probe respects `context.Context` cancellation  
- [ ] Spike binary prints a table: `host:port → status | latency | evidence`  

---

## Test Fixtures

```
# Refused: connect to closed port on localhost
TCPProbe("127.0.0.1", <closed_port>, 5s) → refused

# Timeout: use a non-routable IP (e.g., 192.0.2.1 from TEST-NET-1, RFC 5737)
TCPProbe("192.0.2.1", 443, 500ms) → timeout after 500ms

# DNS error
TCPProbe("this-host-does-not-exist.invalid", 443, 5s) → dns_error
```

---

## Out of Scope

- TLS probing — separate issue in M2
- HTTP probing — separate issue in M2

---

## Definition of Done

PR merged from `spike/m0-tcp-probe` with all AC checked and tests passing on all three OS targets (CI green).

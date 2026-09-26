## What was proven

### Spike A — Effective Configuration ✅
- `shell env > .env > absent` precedence is deterministic and testable
- `present-but-empty` is distinguishable from `absent` (LookupEnv returns ok=true even for empty string)
- `.env` parser handles the documented subset: CRLF, UTF-8 BOM, quoted values, export prefix, inline comments
- `rawValue` is unexported — accidental printing via `%v` or `fmt.Sprintf` is impossible without calling `RawValue()` explicitly
- Secret display rule: `[REDACTED]`, never `sk-****6789`

### Spike B — Deterministic TCP Diagnosis ✅
- Injectable `DialContextFunc` enables hermetic tests with zero OS dependency
- `tcp_refused` / `tcp_timeout` / `tcp_cancelled` / `dns_error` / pass are deterministically distinguishable
- `classifyDialError` produces normalised machine-readable failure kinds
- Human-readable `Evidence` is safe and bounded; no OS-specific strings reach the output

### Spike C — OpenAI-compatible Provider Probe ✅
- `apiKey` is held in an unexported struct field; never passes to any `ProbeResult` field
- `redactSecret()` is applied unconditionally before any response body enters `Evidence`
- `TestProbeModels_EchoedSecret` proves the invariant holds even when a malicious server echoes the key
- `assertNoSecret()` validates both field-by-field and via JSON marshal
- 200/401/403/404/timeout/network-error are distinguishable
- `--api-key` CLI flag does NOT exist

### Spike D — Cross-layer Correlation ✅
- All 4 required scenarios: CONFIG_MISSING_CREDENTIAL, NETWORK_TCP_REFUSED, AUTHENTICATION_FAILED, HEALTHY
- Short-circuit: lower layers block higher layers (DNS fail → Provider skipped)
- TCP timeout diagnosis does NOT claim "this means firewall" — states possible causes only
- Evidence ordering: observations → candidates → fix hint

## What failed

Nothing fundamentally failed. Design adjustments made during the spike:

1. **Value whitespace handling**: Initially forgot to `TrimSpace` unquoted values after `=`. `KEY = value` was producing `" value"`. Fixed.
2. **Test isolation**: Initially used bare `os.Unsetenv` in tests, which doesn't restore on cleanup. Replaced with `unsetEnvForTest` helper that uses `t.Cleanup` for proper restoration.
3. **TLS probe injectability**: The TLS probe uses the injectable `Dial` for the TCP layer but does not have an injectable TLS handshaker. Acceptable for M0; documented as a known limitation.

## What changed in the design

| Original plan | Actual implementation | Reason |
|---|---|---|
| Separate `output.go` file | Added `Report` struct as output container | Cleaner separation of concerns |
| `--api-key` CLI flag forbidden | Enforced — only reads from env/config | Security requirement |
| `sk-****6789` redaction | Changed to `[REDACTED]` | Spec requirement |
| Generic DialContextFunc for TLS too | TCP dial + `tls.Client` wrap | More injectable/testable |

## Known limitations

1. `isConnectionRefused` uses string matching — fragile on future OS versions
2. TLS probe is not fully injectable (TCP layer is, TLS config is not a function)
3. No retry logic
4. No partial success display (only first failing layer shown)
5. JSON schema is minimal — will need versioning strategy for production
6. Generic `shell > .env` precedence is a spike assumption — not universal

## Security invariants

- `ConfigValue.rawValue` is unexported — cannot be accidentally printed
- `redactSecret()` is applied unconditionally on ALL response bodies
- `assertNoSecret()` validates key absence both field-by-field and in JSON marshal output
- `--api-key` flag does not exist in the binary
- `DisplayValue()` is the single safe output path; returns `[REDACTED]` for secret keys

## Test results

All tests pass with:
- `go test ./spike/m0/`
- `go test -race ./spike/m0/`
- `go vet ./spike/m0/`

CI matrix: ubuntu-latest, macos-latest, windows-latest (see Actions tab)

---

> ⚠️ **Do not merge this PR.** M0 is a feasibility spike. Review findings, then design the production architecture from `internal/` upward.

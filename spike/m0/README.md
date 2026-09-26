# M0 Technical Verification Spike

## Purpose

This spike proves four assumptions required before building `aitrouble` v0.1:

| # | Assumption | Proven by |
|---|---|---|
| A | Effective Configuration can be resolved safely and deterministically | `config.go` + `config_test.go` |
| B | Network failure classes (refused / timeout / DNS / cancel) are distinguishable | `network.go` + `network_test.go` |
| C | An OpenAI-compatible provider can be probed without leaking credentials | `provider.go` + `provider_test.go` |
| D | Cross-layer evidence can be correlated into a useful diagnosis | `diagnosis.go` + `diagnosis_test.go` |

## What this spike deliberately does NOT prove

- **Production config precedence**: The `shell env > .env` order is a spike assumption. Different agent runtimes (LM Studio, Claude Desktop, Cursor, VS Code, CI pipelines) may impose different precedence rules. The production Config Engine must be designed from first principles after M0.
- **All .env features**: The parser supports a documented subset. Multiline values, variable interpolation, and escaped quotes are out of scope.
- **MCP probing**: Out of scope for M0.
- **TUI or rich formatting**: Plain terminal output only.
- **Production architecture**: Nothing in `spike/m0/` should be moved to `internal/` without a redesign review.
- **All OpenAI-compatible providers**: Only generic `/v1/models` is tested. Azure, Anthropic, Gemini have different auth flows.

## Known limitations

1. **`isConnectionRefused` uses string matching** — fragile across OS versions. Production code should use `errors.As` with `syscall.Errno` and platform-specific build tags.
2. **TLS probe is not fully injectable** — the `Dial` function is injectable (TCP layer), but the TLS config is passed as a struct field, not a function. This is acceptable for M0 but should be redesigned.
3. **No retry logic** — probes fail on first attempt. A production prober should support configurable retry with backoff.
4. **No partial success** — if DNS succeeds but TCP fails, the diagnosis only shows the TCP failure. A production tool should show the full probe chain.
5. **JSON schema is minimal** — `schema_version: 1` is stable for M0 only. Production JSON schema will need versioning strategy.
6. **Provider probe sends exactly one request** — no session management, no streaming, no retry.

## What SHOULD be carried into production architecture

- ✅ Injectable `DialContextFunc` and `DNSLookupFunc` for hermetic testing
- ✅ `rawValue` unexported to prevent accidental secret leaks
- ✅ `redactSecret()` applied unconditionally before any body enters Evidence
- ✅ `DisplayValue()` as the single safe output path for config values
- ✅ Short-circuit correlation strategy (lower layers evaluated first)
- ✅ Evidence is bounded (size limits on body reads, truncation on long IDs)
- ✅ `FailureKind` is normalised (machine-readable) vs `Evidence` (human-readable)
- ✅ Missing `.env` file is gracefully skipped, but actual filesystem errors (e.g. permission denied) are surfaced
- ✅ HTTP 200 responses are validated against the expected JSON shape (not blindly accepted)
- ✅ Provider probes are bounded by an explicit context deadline

## Decisions that should NOT be carried forward blindly

| Decision | Risk | Recommendation |
|---|---|---|
| `shell env > .env` is universal | Wrong for many agent runtimes | Design Config Engine with explicit runtime profiles |
| `isConnectionRefused` string matching | Fragile on Windows | Use `errors.As(syscall.Errno)` with build tags |
| Single JSON schema for all output | Will break consumers | Design versioned schema with migration path |
| Config keys hardcoded (`ConfigKeys`) | Not extensible | Use plugin/provider registry in production |
| Diagnosis has one primary code | Multi-cause failures need ranked candidates | Production diagnosis should use a probabilistic ranking model |

## Running the spike

```bash
# Run all tests (no internet, no credentials needed)
go test ./spike/m0/

# Run with race detector
go test -race ./spike/m0/

# Run vet
go vet ./spike/m0/

# Run the binary (reads from env / .env)
export OPENAI_API_KEY=your-key
export OPENAI_BASE_URL=https://api.openai.com/v1
go run ./spike/m0/

# JSON output
go run ./spike/m0/ --json

# Test against a local server (skip TLS verification)
export OPENAI_BASE_URL=http://localhost:1234/v1
go run ./spike/m0/ --insecure
```

## Success criteria checklist

- [x] EffectiveConfig precedence proven (shell > .env > absent)
- [x] Empty vs absent proven (Present=true with empty rawValue ≠ absent)
- [x] .env subset documented (see `config.go` ParseDotEnv godoc)
- [x] API secrets never rendered ([REDACTED], not sk-****6789)
- [x] TCP success/refused/timeout/DNS/cancel are deterministic
- [x] Provider /models probe works against local fake server
- [x] 401/403/404/network/timeout are distinguishable
- [x] Exact test key cannot appear in any result/output (assertNoSecret)
- [x] Cross-layer correlation works for 4 required scenarios
- [x] TLS failure (HTTPS) correctly short-circuits the provider probe
- [x] Provider probe enforces context timeout correctly
- [x] Provider probe validates HTTP 200 JSON shape instead of blindly accepting it
- [x] All tests pass with `go test ./spike/m0/` and `go test -race ./spike/m0/`
- [x] No new external dependencies (zero `go.sum` changes)
- [x] No production `internal/` packages created
- [x] No TUI added
- [x] No MCP implementation added
- [x] No LLM added
- [x] No telemetry added

## File structure

```
spike/m0/
├── main.go          — CLI entry point (--env-file, --timeout, --json, --insecure)
├── types.go         — Minimal spike types (ProbeResult, Diagnosis, ConfigValue, ...)
├── config.go        — EffectiveConfig resolver + ParseDotEnv
├── network.go       — NetworkProber with injectable Dial + DNSLookup
├── provider.go      — ProviderProber with secret-redaction invariant
├── diagnosis.go     — Cross-layer correlator (Correlate)
├── output.go        — PrintTerminal + PrintJSON (schema_version: 1)
├── config_test.go   — 12 required config test cases
├── network_test.go  — 5 required network probe tests
├── provider_test.go — Provider tests incl. EchoedSecret invariant
├── diagnosis_test.go— 4 required correlation scenarios
└── testdata/
    ├── config/
    │   ├── basic.env
    │   ├── quoted.env
    │   ├── crlf.env    (CRLF line endings)
    │   └── bom.env     (UTF-8 BOM)
    └── provider/
        ├── models_ok.json
        ├── unauthorized.json
        ├── forbidden.json
        ├── not_found.json
        └── echoed_secret.json  (server echoes API key — must still be redacted)
```

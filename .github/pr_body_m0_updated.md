## Fixed review blockers

1. **Toolchain Mismatch Fixed**: `go.mod` is updated to Go 1.27.0. The GitHub Actions workflow (`.github/workflows/ci.yml`) is updated to use `actions/checkout@v7`, `actions/setup-go@v6`, and Go version `1.27`.
2. **TLS Short-Circuiting**: Modified `main.go` to ensure `networkOK` is only true if TLS succeeds (when applicable). If TLS fails, the provider probe is skipped completely.
3. **Provider Timeout Enforcement**: Added `timeout time.Duration` to `ProviderProber`. `ProbeModels` now strictly wraps the request context with a timeout bounding deadline using `context.WithTimeout(ctx, p.timeout)`. A test `TestProbeModels_Timeout` verifies this works against a slow server.
4. **.env Missing-File Handling**: Updated `loadDotEnv` in `config.go` to explicitly check `os.IsNotExist(err)` to gracefully skip missing files, but return an error for actual filesystem errors (e.g. permission denied).
5. **Provider JSON Shape Validation**: Updated `handleOK` to unmarshal into a `map[string]interface{}` to strictly ensure the `"data"` array key is present. If missing, it now returns `provider_invalid_response` and the diagnosis accurately reflects that the 200 OK did not have the expected shape. A deterministic fixture-based test proves this (`TestProbeModels_OK_InvalidShape`).
6. **No Secret Leaks on Network Errors**: Ensured `assertNoSecret` remains valid even on network and timeout errors (which are covered by tests).
7. **No `--api-key` Added**: Kept CLI interface clean.

## Test results

Local test execution results:
- `go test ./...`: All pass (1.173s)
- `go test -race ./...`: Skipped natively on Windows (CGO is not enabled). The race condition checks run safely in the Ubuntu CI matrix.
- `go vet ./...`: Clean execution.
- `go build ./cmd/aitrouble`: Successful.

## Known limitations

1. `isConnectionRefused` uses string matching — fragile across OS versions. Production code should use `errors.As` with `syscall.Errno` and platform-specific build tags.
2. TLS probe is not fully injectable — the `Dial` function is injectable (TCP layer), but the TLS config is passed as a struct field.
3. No retry logic.
4. No partial success visualization (only first failing layer shown).
5. JSON schema is minimal.
6. The `shell env > .env` precedence is a spike assumption.
7. This M0 Spike does NOT prove production correctness. It verifies technical assumptions in isolation.

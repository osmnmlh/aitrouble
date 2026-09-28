# Organic validation laboratory

`run-organic-tests.ps1` builds and tests a **release-style**, explicitly versioned Windows binary from the current checkout. It is not a unit-test wrapper: each test runs the binary in an isolated project, with real DNS/TCP/TLS/HTTP behavior and an independent fixture oracle.

## Run

```powershell
.\organic-tests\run-organic-tests.ps1
# replay exactly
.\organic-tests\run-organic-tests.ps1 -Seed 123456
```

Every invocation creates a unique preserved lab at `organic-tests/lab/<timestamp>-seed-<seed>/`; it never removes a previous lab or edits user MCP configuration, user environment variables, or the source checkout.

Reports are written both to the lab and to `organic-tests/reports/`:

- `ORGANIC_TEST_REPORT.md`
- `organic-test-summary.json`
- `FAILURES.md`
- `MUTATION_TEST_REPORT.md`
- `SEED_INDEX.json`

The runner uses a local OpenAI-compatible HTTP service, direct socket/DNS checks, local TLS fixtures, isolated `%USERPROFILE%`/`%APPDATA%`, per-run secret canaries, and a disposable source copy for mutation tests. Public clones of `assistant-ui/assistant-ui-starter-minimal` and `vercel/chatbot` are recorded as optional organic evidence; deterministic controlled tests do not depend on them.

`PASS` means every assertion for that scenario passed. `FAIL`, `BLOCKED`, and `FLAKY` remain distinct in the report and cause the overall release verdict to be non-PASS. A mutation scenario is `PASS` only when the harness actually rejects its intentionally broken disposable binary.

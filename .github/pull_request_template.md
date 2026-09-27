## Related Issue

Closes #<!-- Issue number -->

---

## Summary of Changes

<!-- A concise bullet-list of what this PR does. -->

- 
- 

---

## Change Type

<!-- Check all that apply -->

- [ ] 🐛 Bug fix (non-breaking change that fixes an issue)
- [ ] 🚀 New feature (non-breaking change that adds functionality)
- [ ] 💥 Breaking change (fix or feature that would cause existing behavior to change)
- [ ] 🔧 Refactor (no functional change)
- [ ] 📝 Documentation update
- [ ] 🧪 Test-only change
- [ ] 🏗️ CI/Build improvement

---

## Testing

<!-- Describe the tests you've added or modified. -->

- [ ] New unit tests added in `_test.go` files
- [ ] New integration/fixture tests added under `testdata/`
- [ ] Existing tests pass: `go test -v -race ./...`
- [ ] Manually tested with a real endpoint (redacted output attached below, if applicable)

<details>
<summary>Manual test output (redacted)</summary>

```
<!-- Paste redacted aitrouble doctor output here -->
```

</details>

---

## Breaking Change Checklist

- [ ] This PR does **not** change any public API, CLI flags, or output format
- [ ] If it does change CLI flags or output format, the CHANGELOG and README are updated

---

## Security Checklist

> `aitrouble` is a security-sensitive tool. Every PR must pass these checks.

- [ ] **Zero secrets leaked** — No API keys, tokens, or credentials appear in source code, test fixtures, log output, or error messages
- [ ] **Read-only preserved** — This PR does not add any write operations to user config files, shell environment, or filesystem (outside of designated output flags)
- [ ] **Secret redaction tested** — If this PR touches config reading or HTTP response handling, the redaction logic has been verified (unit test or manual check)
- [ ] **No new external dependencies** — Or, if a new dependency is added, it has been discussed in the linked issue and added with justification in `go.mod`
- [ ] **No telemetry added** — No analytics, tracking, or beaconing of any kind has been introduced

---

## Documentation Checklist

- [ ] Updated `docs/architecture.md` when architecture/domain behavior changed
- [ ] Updated `docs/use-cases.md` when supported/planned usage changed
- [ ] Updated `docs/requirements.md` when requirements/scope changed
- [ ] No documentation claims functionality that is not implemented

---

## Reviewer Notes

<!-- Anything you want reviewers to pay special attention to. -->

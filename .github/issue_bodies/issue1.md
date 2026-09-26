## Spike Goal

Prove that `aitrouble` can deterministically resolve the **Effective Configuration** by merging config sources in the correct precedence order:

```
shell env  >  .env file  >  project config file  >  defaults
```

This spike lives in `spike/m0-effective-config/main.go` and must not affect any other package.

---

## Context

When a developer runs `aitrouble doctor`, the first thing the tool does is build an **EffectiveConfig**: a merged, precedence-aware view of all config sources. Getting this right is critical — showing the wrong base URL or API key in the diagnostic output would mislead the user completely.

Known edge cases to handle:
- Shell env shadows `.env` (e.g., `export OPENAI_API_KEY=x` overrides `.env` value)  
- `.env` file may not exist (graceful skip, no error)  
- Values may be empty strings (different from "not set")  
- Key names may vary (`OPENAI_BASE_URL` vs `OPENAI_API_BASE`)

---

## Acceptance Criteria

- [ ] `EffectiveConfig` struct holds `Value string`, `Source string` (e.g., `"shell env"`, `".env file"`, `"default"`), and `Redacted string` per key  
- [ ] Shell env takes precedence over `.env` file — proven by a unit test that sets both  
- [ ] `.env` file is parsed without any external library (pure stdlib)  
- [ ] Missing `.env` file is handled gracefully (no panic, no error surfaced to user)  
- [ ] Empty string in shell env is treated as "set but empty" — documented behavior  
- [ ] API keys are redacted: `sk-abcdefghijklmnop` → `sk-•••••••••••••nop` (last 4 chars visible)  
- [ ] Unit tests cover: shell-only, .env-only, both present (shell wins), neither present (default used)  
- [ ] Spike runs standalone: `go run ./spike/m0-effective-config/` prints EffectiveConfig table to stdout  

---

## Out of Scope

- Reading project config files (pyproject.toml, package.json) — M1
- Writing or modifying any config source — never in scope
- Network probes of any kind

---

## Definition of Done

PR merged to `main` from branch `spike/m0-effective-config` with all AC checked.

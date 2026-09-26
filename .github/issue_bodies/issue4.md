## Spike Goal

Build a minimal prototype in `spike/main.go` that **correlates results across all three spike probes** (config, TCP, provider) to produce a single human-readable diagnosis — exactly as `aitrouble doctor` will do in M3.

This spike is the integration test of M0: if all three previous spikes are done, this one wires them together.

---

## Context

Each spike (config, TCP, provider) produces a `ProbeResult`. The correlation layer's job is to:

1. Run probes in the correct order (config → TCP → provider)
2. **Stop early** if a lower-layer probe fails (no point probing provider if TCP is down)
3. Identify the **first failing layer** as the diagnosis
4. Emit a human-readable diagnosis with a concrete fix hint

This is the core value proposition of `aitrouble`: not just showing errors, but identifying *which link in the chain broke*.

---

## Acceptance Criteria

- [ ] `spike/main.go` imports the three spike packages and runs them sequentially  
- [ ] Implements `Correlate(results []ProbeResult) Diagnosis` function  
- [ ] `Diagnosis` struct has: `FailingLayer string`, `Summary string`, `FixHint string`, `Evidence []string`  
- [ ] **Short-circuit logic**: if TCP fails → skip provider probe, set `FailingLayer = "network"`, suggest network fix  
- [ ] **Config-only issue**: if EffectiveConfig has no `OPENAI_API_KEY` → `FailingLayer = "config"`, fix hint = "Set OPENAI_API_KEY"  
- [ ] **Provider auth failure**: if TCP passes but provider returns 401 → `FailingLayer = "provider"`, fix hint = "Regenerate API key"  
- [ ] **All pass**: `FailingLayer = ""`, Summary = "All checks passed"  
- [ ] Terminal output matches the demo format in README (with ✓/✗/– symbols and numbered sections)  
- [ ] No secrets in output (inherits redaction from component spikes)  
- [ ] Unit test covers all four diagnosis paths above  

---

## Expected Output Format

```
 aitrouble v0.0.0-spike  •  Find where your AI integration breaks
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[1/3] Effective Configuration
  ✓  OPENAI_API_KEY   = sk-•••••••••••••••••••oXyZ  (shell env)
  ✓  OPENAI_BASE_URL  = https://api.openai.com/v1   (.env file)

[2/3] Network Probe
  ✓  DNS   api.openai.com → 104.18.7.192  (12ms)
  ✓  TCP   104.18.7.192:443               (23ms)

[3/3] Provider Probe
  ✗  GET /v1/models → 401 Unauthorized
     Evidence: {"error":{"code":"invalid_api_key"}}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 DIAGNOSIS  Provider › invalid_api_key
 FIX HINT   Regenerate your API key at https://platform.openai.com/api-keys
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

## Dependencies

Depends on: #1, #2, #3 (all three spike packages must be importable)

---

## Definition of Done

PR merged from `spike/m0-correlation` with all AC checked and a demo GIF or terminal recording attached to the PR description.

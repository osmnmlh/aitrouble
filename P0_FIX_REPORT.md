# P0 Defect Fixes Report

## Starting Point
Starting Git Hash: `b644b61a8e5c87887bf3197150c1d98f6d9bc857` (from branch `chore/v0.1.0-release-polish`)

## Hypotheses Tested and Proven
1. **D1 (Empty config mishandling):** Display formatting and value semantics are mixed in one method. The fix separates display formatting from the raw value and makes emptiness checks use the raw value via an explicit `IsEmpty()` accessor.
2. **D2 (Dotenv parser inline comments):** `stripInlineComment` naively returns early if the first character is a quote, skipping comment stripping for quoted values. The fix implements proper state tracking for single and double quotes to identify and strip comments only when they are outside of quoted sections and preceded by whitespace.
3. **D3 (Connection close classified as Unknown):** The transport-error classifier falls through to a default branch for EOF / reset errors, and `diagnoseFailure` does not handle `http_error`, causing it to map to `Unknown`. The fix explicitly classifies EOF and reset errors using `errors.Is`/`errors.As` on typed errors, returns `http_error` in the provider classifier, and adds `http_error` handling to `diagnoseFailure`.

## Unit Test Outputs (Reproduction Phase)

### D1 Reproduction
```text
=== RUN   TestDoctor_EmptyBaseURL
    doctor_d1_test.go:19: expected Missing OPENAI_BASE_URL in stderr, got:
        STDOUT:
        aitrouble doctor
        Find where your AI integration breaks.
        ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        
        [1/4] Effective Configuration
          ⚠  OPENAI_API_KEY   (not set)
          ✓  OPENAI_BASE_URL  ""                source: shell
        
        [2/4] Network Probes
          ✗  Parse ""                                 [invalid_url]
             Evidence: unsupported URL scheme
        
        [3/4] Provider Probe
          –  OpenAI /models                          
             Evidence: skipped due to network failure
        
        [4/4] Local MCP
          –  No supported MCP configuration detected
        
        ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
         DIAGNOSIS  The chain breaks at: Network
         SUMMARY    The configured base URL could not be parsed into a supported target.
         FIX        Check the OPENAI_BASE_URL format in your configuration.
        ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        
        STDERR:
--- FAIL: TestDoctor_EmptyBaseURL (0.00s)
FAIL
```

### D2 Reproduction
```text
=== RUN   TestParseDotEnv_Variants
=== RUN   TestParseDotEnv_Variants/KEY="value"_#_comment
    config_d2_test.go:77: key "KEY": expected "value", got "\"value\" # comment"
=== RUN   TestParseDotEnv_Variants/KEY='value'_#_comment
    config_d2_test.go:77: key "KEY": expected "value", got "'value' # comment"
=== RUN   TestParseDotEnv_Variants/KEY=value_#_comment
=== RUN   TestParseDotEnv_Variants/KEY=http://h/p#frag
    config_d2_test.go:77: key "KEY": expected "http://h/p#frag", got "http://h/p"
=== RUN   TestParseDotEnv_Variants/KEY="a_#_b"
=== RUN   TestParseDotEnv_Variants/KEY=""
=== RUN   TestParseDotEnv_Variants/KEY=
=== RUN   TestParseDotEnv_Variants/export_KEY=value_with_whitespace
=== RUN   TestParseDotEnv_Variants/BOM_CRLF
=== RUN   TestParseDotEnv_Variants/missing_trailing_newline
--- FAIL: TestParseDotEnv_Variants (0.00s)
    --- FAIL: TestParseDotEnv_Variants/KEY="value"_#_comment (0.00s)
    --- FAIL: TestParseDotEnv_Variants/KEY='value'_#_comment (0.00s)
    --- PASS: TestParseDotEnv_Variants/KEY=value_#_comment (0.00s)
    --- FAIL: TestParseDotEnv_Variants/KEY=http://h/p#frag (0.00s)
    --- PASS: TestParseDotEnv_Variants/KEY="a_#_b" (0.00s)
    --- PASS: TestParseDotEnv_Variants/KEY="" (0.00s)
    --- PASS: TestParseDotEnv_Variants/KEY= (0.00s)
    --- PASS: TestParseDotEnv_Variants/export_KEY=value_with_whitespace (0.00s)
    --- PASS: TestParseDotEnv_Variants/BOM_CRLF (0.00s)
    --- PASS: TestParseDotEnv_Variants/missing_trailing_newline (0.00s)
FAIL
```

### D3 Reproduction
```text
=== RUN   TestDiagnoseFailure_D3
    diagnosis_d3_test.go:15: expected Provider › HTTP, got Unknown
--- FAIL: TestDiagnoseFailure_D3 (0.00s)
FAIL
```

## Organic Test Runner Output (Final Passing Run)
```text
Organic lab C:\Users\melih\.gemini\antigravity-ide\scratch\aitrouble\organic-tests\lab\20260928T181722Z-seed-42
Candidate aitrouble organic-b644b61-dirty (15660bb159245aed8d511362cb690caac4bbf650b946beb08e3b4686b60e76f0)
Verdict: PASS. Reports: C:\Users\melih\.gemini\antigravity-ide\scratch\aitrouble\organic-tests\reports
```

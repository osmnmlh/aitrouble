# Requirements

## 1. Product Goal

To provide a reliable, single-binary, read-only diagnostic tool that developers can use to figure out why their AI integration is failing without leaking their credentials or reading endless logs.

## 2. Functional Requirements

| ID | Description | Status |
|---|---|---|
| **FR-01** | Effective configuration resolution | Status: Implemented |
| **FR-02** | Secret-safe configuration representation | Status: Implemented |
| **FR-03** | DNS probing | Status: Implemented |
| **FR-04** | TCP probing | Status: Implemented |
| **FR-05** | TLS probing | Status: Implemented |
| **FR-06** | Provider `/models` probing | Status: Implemented |
| **FR-07** | Probe result normalization | Status: Implemented |
| **FR-08** | Short-circuit behavior | Status: Implemented |
| **FR-09** | Human-readable diagnostic output | Status: Implemented |
| **FR-10** | Cross-layer diagnosis | Status: Implemented |
| **FR-11** | CLI doctor command | Status: Implemented |
| **FR-12** | MCP config discovery (static) | Status: Implemented |
| **FR-12B** | MCP process probing (M5B) | Status: Planned |
| **FR-13** | JSON output | Status: Planned |
| **FR-14** | Exit-code semantics | Status: Implemented |

## 3. Non-Functional Requirements

- **NFR-01 Local-first:** Diagnostics must execute securely from the user's local machine without relying on a remote observability suite. *(Status: Implemented)*
- **NFR-02 Zero telemetry:** The binary will never collect usage metrics, analytics, or trace data. *(Status: Implemented)*
- **NFR-03 Read-only behavior:** We inspect but do not mutate configuration or filesystem elements. *(Status: Implemented)*
- **NFR-04 Secret safety:** Redaction patterns must guarantee no leakage of credentials. *(Status: Implemented)*
- **NFR-05 Deterministic testing:** Tests run cleanly without a real network connection using standard library mocks. *(Status: Implemented)*
- **NFR-06 Timeout bounded network operations:** All network calls have strict timeout limits. *(Status: Implemented)*
- **NFR-07 Cross-platform support:** Native Windows, Linux, and macOS behaviors supported. *(Status: Implemented)*
- **NFR-08 Standard library / minimal dependency policy:** No Viper, Cobra, or giant HTTP frameworks. Standard library is paramount. *(Status: Implemented)*
- **NFR-09 Single binary:** Output will be a single executable. *(Status: Implemented)*
- **NFR-10 Maintainable small codebase:** Architecture is structured for fast reading. *(Status: Implemented)*

## 4. Security Requirements

- **SEC-01** API keys must never appear in `ProbeResult` output fields.
- **SEC-02** API keys must never appear in normal terminal evidence.
- **SEC-03** API keys must only be sent to the configured provider endpoint.
- **SEC-04** Response bodies must not directly become raw evidence.
- **SEC-05** Secret-echo scenarios must be tested.
- **SEC-06** No telemetry.
- **SEC-07** No persistent credential storage.
- **SEC-08** No `--api-key` CLI flag.

## 5. Current Scope

| Capability                    | Status        |
| ----------------------------- | ------------- |
| Effective Configuration       | ✅ Implemented |
| .env parsing                  | ✅ Implemented |
| DNS probe                     | ✅ Implemented |
| TCP probe                     | ✅ Implemented |
| TLS probe                     | ✅ Implemented |
| OpenAI-compatible `/models`     | ✅ Implemented |
| Secret-safe provider handling | ✅ Implemented |
| Production correlation        | ✅ Implemented |
| Doctor CLI                    | ✅ Implemented |
| Human diagnostic report       | ✅ Implemented |
| Exit-code semantics           | ✅ Implemented |
| Local MCP config discovery    | ✅ Implemented |
| MCP process probing (M5B)     | ⏳ Planned     |
| JSON output                   | ⏳ Planned     |
| TUI                           | ⏳ Planned     |
| LLM diagnosis                 | ⏳ Planned     |

## 6. Out of Scope

- Packet capture, ICMP, and MTU diagnostics.
- Changing user configurations.
- Resolving complex Python/Node virtual environment pathing.
- Generic CI/CD telemetry tracking.

## 7. Future Requirements

*(Planned for future iteration)*
- Detailed cross-layer diagnosis tree formatting.
- Extensible provider profiles (Anthropic, Gemini, local Ollama).
- End-to-end Local MCP testing.

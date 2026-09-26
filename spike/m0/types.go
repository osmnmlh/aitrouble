// Package main implements the aitrouble M0 technical verification spike.
//
// This is a DISPOSABLE engineering spike. Its purpose is to prove four assumptions:
//  1. Effective Configuration can be resolved safely and deterministically.
//  2. Network failure classes can be detected deterministically.
//  3. An OpenAI-compatible provider can be probed without leaking credentials.
//  4. Cross-layer evidence can be correlated into a useful diagnosis.
//
// Do NOT refactor these types into the production internal/ package hierarchy.
// The production data model will be designed after M0 findings are analysed.
package main

import "time"

// CheckStatus is the outcome of a single diagnostic probe.
type CheckStatus string

const (
	StatusPass CheckStatus = "pass"
	StatusFail CheckStatus = "fail"
	StatusSkip CheckStatus = "skip"
)

// ProbeResult holds the result of one diagnostic probe.
//
// Evidence must be safe to print: no secrets, no raw API keys, no credentials.
// All string fields are bounded in length.
type ProbeResult struct {
	Name        string        // human-readable probe name
	Status      CheckStatus   // pass / fail / skip
	FailureKind string        // normalised machine-readable failure class (empty on pass)
	Latency     time.Duration // wall-clock duration of the probe
	Evidence    []string      // safe, redacted, bounded observations
}

// Diagnosis is the final correlated conclusion from all probe layers.
type Diagnosis struct {
	Code       string      // machine-readable diagnosis code (see diagnosis.go)
	Summary    string      // one-sentence human summary
	Candidates []Candidate // ranked list of possible causes
	Evidence   []string    // key observations supporting this diagnosis
	FixHint    string      // concrete next step for the developer
}

// Candidate is one possible cause within a Diagnosis.
type Candidate struct {
	Code       string // short machine-readable label
	Confidence string // "high" | "medium" | "low"
	Reason     string // human explanation
}

// ConfigValue represents one resolved configuration entry.
//
// The rawValue field is intentionally unexported. It must never appear in
// Evidence, terminal output, JSON output, logs, or error messages.
// Use DisplayValue() for all output. Use RawValue() only for internal
// probe logic where the actual value is needed (e.g., provider auth).
type ConfigValue struct {
	Key     string // configuration key name
	Present bool   // true if the key exists in any source (even if empty)
	Source  string // "shell" | ".env" | "absent"
	Secret  bool   // true if this key should never be displayed

	rawValue string // NEVER export, NEVER include in any output
}

// RawValue returns the actual value for internal probe use.
//
// IMPORTANT: never pass the return value of RawValue() to Evidence, fmt.Sprintf
// for output, json.Marshal for output, or any logging call.
func (cv ConfigValue) RawValue() string { return cv.rawValue }

// DisplayValue returns a safe representation for terminal and JSON output.
//
//   - absent key              → "absent"
//   - secret key (any value)  → "[REDACTED]"
//   - non-secret empty value  → "(empty)"
//   - non-secret non-empty    → the raw value
func (cv ConfigValue) DisplayValue() string {
	if !cv.Present {
		return "absent"
	}
	if cv.Secret {
		return "[REDACTED]"
	}
	if cv.rawValue == "" {
		return "(empty)"
	}
	return cv.rawValue
}

// ProbeSet collects all probe results for cross-layer correlation.
// A nil pointer means that probe was not run (skipped).
type ProbeSet struct {
	Config   map[string]ConfigValue
	DNS      *ProbeResult
	TCP      *ProbeResult
	TLS      *ProbeResult
	Provider *ProbeResult
}

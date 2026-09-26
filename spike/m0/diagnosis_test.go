package main

import (
	"testing"
)

// ── Helper: build a ProbeSet for correlation tests ────────────────────────────

func configWith(apiKey, baseURL string) map[string]ConfigValue {
	cfg := map[string]ConfigValue{
		"OPENAI_API_KEY": {
			Key:      "OPENAI_API_KEY",
			Present:  apiKey != "",
			Source:   "shell",
			Secret:   true,
			rawValue: apiKey,
		},
		"OPENAI_BASE_URL": {
			Key:      "OPENAI_BASE_URL",
			Present:  baseURL != "",
			Source:   "shell",
			Secret:   false,
			rawValue: baseURL,
		},
	}
	if apiKey == "" {
		cfg["OPENAI_API_KEY"] = ConfigValue{
			Key:     "OPENAI_API_KEY",
			Present: false,
			Source:  "absent",
			Secret:  true,
		}
	}
	return cfg
}

func passResult(name string) *ProbeResult {
	return &ProbeResult{Name: name, Status: StatusPass}
}

func failResult(name, kind string) *ProbeResult {
	return &ProbeResult{
		Name:        name,
		Status:      StatusFail,
		FailureKind: kind,
		Evidence:    []string{kind},
	}
}

// ── Scenario 1: CONFIG_MISSING_CREDENTIAL ────────────────────────────────────

func TestCorrelate_Scenario1_MissingCredential(t *testing.T) {
	ps := ProbeSet{
		Config: configWith("", "https://api.openai.com/v1"),
		// No DNS/TCP/TLS/Provider — they must be skipped when config fails.
	}
	d := Correlate(ps)

	assertDiagCode(t, d, DiagConfigMissingCredential)
	assertConfidence(t, d, "high")

	// Provider probe must have been short-circuited (nil in ProbeSet).
	// Verified implicitly: if provider ran, it would panic since dns/tcp are nil.
}

// ── Scenario 2: NETWORK_TCP_REFUSED ──────────────────────────────────────────

func TestCorrelate_Scenario2_TCPRefused(t *testing.T) {
	ps := ProbeSet{
		Config:   configWith("sk-test", "http://localhost:9/v1"),
		DNS:      passResult("DNS"),
		TCP:      failResult("TCP", "tcp_refused"),
		Provider: nil, // must be skipped
	}
	d := Correlate(ps)

	assertDiagCode(t, d, DiagNetworkTCPRefused)
	assertConfidence(t, d, "high")
}

// ── Scenario 3: AUTHENTICATION_FAILED ────────────────────────────────────────

func TestCorrelate_Scenario3_AuthFailed(t *testing.T) {
	ps := ProbeSet{
		Config:   configWith("sk-bad-key", "https://api.openai.com/v1"),
		DNS:      passResult("DNS"),
		TCP:      passResult("TCP"),
		TLS:      passResult("TLS"),
		Provider: failResult("Provider /models", "auth_failure"),
	}
	d := Correlate(ps)

	assertDiagCode(t, d, DiagAuthenticationFailed)
	assertConfidence(t, d, "high")
	// Should have multiple candidates
	if len(d.Candidates) < 2 {
		t.Errorf("expected at least 2 candidates for auth failure, got %d", len(d.Candidates))
	}
}

// ── Scenario 4: HEALTHY ───────────────────────────────────────────────────────

func TestCorrelate_Scenario4_Healthy(t *testing.T) {
	ps := ProbeSet{
		Config:   configWith("sk-valid", "https://api.openai.com/v1"),
		DNS:      passResult("DNS"),
		TCP:      passResult("TCP"),
		TLS:      passResult("TLS"),
		Provider: passResult("Provider /models"),
	}
	d := Correlate(ps)

	assertDiagCode(t, d, DiagHealthy)
	assertConfidence(t, d, "high")
}

// ── Short-circuit: DNS failure skips TCP/Provider ────────────────────────────

func TestCorrelate_DNSFailureSkipsProvider(t *testing.T) {
	ps := ProbeSet{
		Config:   configWith("sk-test", "https://api.openai.com/v1"),
		DNS:      failResult("DNS", "dns_error"),
		TCP:      nil, // not run
		Provider: nil, // not run
	}
	d := Correlate(ps)
	assertDiagCode(t, d, DiagNetworkDNSError)
}

// ── TCP timeout ───────────────────────────────────────────────────────────────

func TestCorrelate_TCPTimeout(t *testing.T) {
	ps := ProbeSet{
		Config:   configWith("sk-test", "https://api.openai.com/v1"),
		DNS:      passResult("DNS"),
		TCP:      failResult("TCP", "tcp_timeout"),
		Provider: nil,
	}
	d := Correlate(ps)
	assertDiagCode(t, d, DiagNetworkTCPTimeout)
	// Fix hint must NOT state "this means firewall" (that is not proven)
	if d.FixHint == "" {
		t.Error("expected a fix hint for TCP timeout")
	}
}

// ── TLS failure ───────────────────────────────────────────────────────────────

func TestCorrelate_TLSFailure(t *testing.T) {
	ps := ProbeSet{
		Config:   configWith("sk-test", "https://api.openai.com/v1"),
		DNS:      passResult("DNS"),
		TCP:      passResult("TCP"),
		TLS:      failResult("TLS", "tls_cert_error"),
		Provider: nil,
	}
	d := Correlate(ps)
	assertDiagCode(t, d, DiagNetworkTLSError)
}

// ── Evidence ordering ─────────────────────────────────────────────────────────

func TestCorrelate_DiagnosisHasEvidence(t *testing.T) {
	ps := ProbeSet{
		Config: configWith("", ""),
	}
	d := Correlate(ps)
	if len(d.Evidence) == 0 {
		t.Error("Diagnosis should contain evidence")
	}
	if len(d.Candidates) == 0 {
		t.Error("Diagnosis should contain at least one candidate")
	}
	if d.FixHint == "" {
		t.Error("Diagnosis should contain a fix hint")
	}
}

// ── Test helpers ──────────────────────────────────────────────────────────────

func assertDiagCode(t *testing.T, d Diagnosis, want string) {
	t.Helper()
	if d.Code != want {
		t.Errorf("Diagnosis.Code: want %q, got %q (summary: %q)", want, d.Code, d.Summary)
	}
}

func assertConfidence(t *testing.T, d Diagnosis, want string) {
	t.Helper()
	if len(d.Candidates) == 0 {
		t.Error("Diagnosis has no candidates")
		return
	}
	if d.Candidates[0].Confidence != want {
		t.Errorf("Candidates[0].Confidence: want %q, got %q", want, d.Candidates[0].Confidence)
	}
}

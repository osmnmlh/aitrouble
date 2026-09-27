package diagnosis

import (
	"strings"
	"testing"

	"github.com/osmnmlh/aitrouble/internal/core"
)

// Table-driven test covering all major failure kinds with FailingLayer + Summary + FixHint assertions.
func TestCorrelate_AllFailureKinds(t *testing.T) {
	tests := []struct {
		failureKind     string
		expectedLayer   string
		summaryContains string
		fixHintContains string
	}{
		{"invalid_url", "Network", "base URL", "OPENAI_BASE_URL"},
		{"dns_error", "Network › DNS", "DNS resolution failed", "hostname"},
		{"dns_timeout", "Network › DNS", "DNS resolution failed", "hostname"},
		{"tcp_refused", "Network › TCP", "actively refused", "service is listening"},
		{"tcp_timeout", "Network › TCP", "timed out", "firewall"},
		{"tls_cert_error", "Network › TLS", "certificate", "certificate"},
		{"tls_timeout", "Network › TLS", "TLS", "TLS"},
		{"tls_error", "Network › TLS", "TLS", "TLS"},
		{"auth_failure", "Provider › Authentication", "network path succeeded", "OPENAI_API_KEY"},
		{"route_not_found", "Provider › /models", "404", "OPENAI_BASE_URL"},
		{"rate_limited", "Provider", "429", "quota"},
		{"provider_server_error", "Provider", "5xx", "server"},
		{"provider_invalid_response", "Provider › Response", "200", "/models"},
		{"provider_http_error", "Provider", "HTTP", ""},
		{"http_timeout", "Provider › HTTP", "timed out", ""},
		{"http_cancelled", "Provider › HTTP", "timed out", ""},
	}

	for _, tt := range tests {
		t.Run(tt.failureKind, func(t *testing.T) {
			results := []core.ProbeResult{
				{Name: "probe", Status: core.StatusFail, FailureKind: tt.failureKind},
			}
			diag := Correlate(results)

			if diag.FailingLayer != tt.expectedLayer {
				t.Errorf("FailingLayer: want %q, got %q", tt.expectedLayer, diag.FailingLayer)
			}
			if tt.summaryContains != "" && !strings.Contains(strings.ToLower(diag.Summary), strings.ToLower(tt.summaryContains)) {
				t.Errorf("Summary: expected to contain %q, got %q", tt.summaryContains, diag.Summary)
			}
			if tt.fixHintContains != "" && !strings.Contains(diag.FixHint, tt.fixHintContains) {
				t.Errorf("FixHint: expected to contain %q, got %q", tt.fixHintContains, diag.FixHint)
			}
			if diag.Summary == "" {
				t.Errorf("Summary must not be empty for failure kind %q", tt.failureKind)
			}
			if diag.FixHint == "" {
				t.Errorf("FixHint must not be empty for failure kind %q", tt.failureKind)
			}
		})
	}
}

// TestCorrelate_AllPass verifies healthy diagnosis.
func TestCorrelate_AllPass(t *testing.T) {
	results := []core.ProbeResult{
		{Name: "DNS", Status: core.StatusPass},
		{Name: "TCP", Status: core.StatusPass},
		{Name: "TLS", Status: core.StatusPass},
		{Name: "Provider", Status: core.StatusPass},
	}
	diag := Correlate(results)
	if diag.FailingLayer != "None" {
		t.Errorf("expected None, got %q", diag.FailingLayer)
	}
	if diag.Summary == "" {
		t.Error("Summary should not be empty on healthy result")
	}
}

// TestCorrelate_FirstFailWins verifies the first broken layer is diagnosed.
func TestCorrelate_FirstFailWins(t *testing.T) {
	// DNS fail → must NOT produce Provider diagnosis
	results := []core.ProbeResult{
		{Name: "DNS", Status: core.StatusFail, FailureKind: "dns_error"},
		{Name: "TCP", Status: core.StatusSkip},
		{Name: "TLS", Status: core.StatusSkip},
		{Name: "Provider", Status: core.StatusSkip},
	}
	diag := Correlate(results)
	if !strings.HasPrefix(diag.FailingLayer, "Network") {
		t.Errorf("DNS fail should produce Network layer diagnosis, got %q", diag.FailingLayer)
	}

	// TLS fail → must NOT produce Provider diagnosis
	results2 := []core.ProbeResult{
		{Name: "DNS", Status: core.StatusPass},
		{Name: "TCP", Status: core.StatusPass},
		{Name: "TLS", Status: core.StatusFail, FailureKind: "tls_cert_error"},
		{Name: "Provider", Status: core.StatusSkip},
	}
	diag2 := Correlate(results2)
	if !strings.HasPrefix(diag2.FailingLayer, "Network") {
		t.Errorf("TLS fail should produce Network layer diagnosis, got %q", diag2.FailingLayer)
	}

	// Provider auth → must produce Provider diagnosis (network passes)
	results3 := []core.ProbeResult{
		{Name: "DNS", Status: core.StatusPass},
		{Name: "TCP", Status: core.StatusPass},
		{Name: "TLS", Status: core.StatusPass},
		{Name: "Provider", Status: core.StatusFail, FailureKind: "auth_failure"},
	}
	diag3 := Correlate(results3)
	if diag3.FailingLayer != "Provider › Authentication" {
		t.Errorf("Provider auth fail should produce Provider › Authentication, got %q", diag3.FailingLayer)
	}
}

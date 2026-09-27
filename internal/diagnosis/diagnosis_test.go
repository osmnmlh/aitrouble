package diagnosis

import (
	"testing"

	"github.com/osmnmlh/aitrouble/internal/core"
)

func TestCorrelate(t *testing.T) {
	tests := []struct {
		name          string
		results       []core.ProbeResult
		expectedLayer string
		expectedFail  bool
	}{
		{
			name: "all pass",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusPass},
				{Name: "TLS", Status: core.StatusPass},
				{Name: "Provider", Status: core.StatusPass},
			},
			expectedLayer: "None",
			expectedFail:  false,
		},
		{
			name: "invalid url",
			results: []core.ProbeResult{
				{Name: "Parse", Status: core.StatusFail, FailureKind: "invalid_url"},
			},
			expectedLayer: "Network",
			expectedFail:  true,
		},
		{
			name: "dns failure",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusFail, FailureKind: "dns_error"},
				{Name: "TCP", Status: core.StatusSkip},
				{Name: "TLS", Status: core.StatusSkip},
				{Name: "Provider", Status: core.StatusSkip},
			},
			expectedLayer: "Network › DNS",
			expectedFail:  true,
		},
		{
			name: "tcp refusal",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusFail, FailureKind: "tcp_refused"},
				{Name: "TLS", Status: core.StatusSkip},
				{Name: "Provider", Status: core.StatusSkip},
			},
			expectedLayer: "Network › TCP",
			expectedFail:  true,
		},
		{
			name: "tcp timeout",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusFail, FailureKind: "tcp_timeout"},
				{Name: "TLS", Status: core.StatusSkip},
				{Name: "Provider", Status: core.StatusSkip},
			},
			expectedLayer: "Network › TCP",
			expectedFail:  true,
		},
		{
			name: "tls cert failure",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusPass},
				{Name: "TLS", Status: core.StatusFail, FailureKind: "tls_cert_error"},
				{Name: "Provider", Status: core.StatusSkip},
			},
			expectedLayer: "Network › TLS",
			expectedFail:  true,
		},
		{
			name: "tls timeout",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusPass},
				{Name: "TLS", Status: core.StatusFail, FailureKind: "tls_timeout"},
				{Name: "Provider", Status: core.StatusSkip},
			},
			expectedLayer: "Network › TLS",
			expectedFail:  true,
		},
		{
			name: "provider 401 auth",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusPass},
				{Name: "TLS", Status: core.StatusPass},
				{Name: "Provider", Status: core.StatusFail, FailureKind: "auth_failure"},
			},
			expectedLayer: "Provider › Authentication",
			expectedFail:  true,
		},
		{
			name: "provider 404 route not found",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusPass},
				{Name: "TLS", Status: core.StatusPass},
				{Name: "Provider", Status: core.StatusFail, FailureKind: "route_not_found"},
			},
			expectedLayer: "Provider › /models",
			expectedFail:  true,
		},
		{
			name: "provider 429 rate limit",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusPass},
				{Name: "TLS", Status: core.StatusPass},
				{Name: "Provider", Status: core.StatusFail, FailureKind: "rate_limited"},
			},
			expectedLayer: "Provider",
			expectedFail:  true,
		},
		{
			name: "provider 5xx server error",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusPass},
				{Name: "TLS", Status: core.StatusPass},
				{Name: "Provider", Status: core.StatusFail, FailureKind: "provider_server_error"},
			},
			expectedLayer: "Provider",
			expectedFail:  true,
		},
		{
			name: "provider invalid response",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusPass},
				{Name: "TCP", Status: core.StatusPass},
				{Name: "TLS", Status: core.StatusPass},
				{Name: "Provider", Status: core.StatusFail, FailureKind: "provider_invalid_response"},
			},
			expectedLayer: "Provider › Response",
			expectedFail:  true,
		},
		{
			name: "unknown failure",
			results: []core.ProbeResult{
				{Name: "DNS", Status: core.StatusFail, FailureKind: "random_mystery_error"},
			},
			expectedLayer: "Unknown",
			expectedFail:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diag := Correlate(tt.results)
			if diag.FailingLayer != tt.expectedLayer {
				t.Errorf("expected failing layer %q, got %q", tt.expectedLayer, diag.FailingLayer)
			}
			isFail := diag.FailingLayer != "None"
			if isFail != tt.expectedFail {
				t.Errorf("expected fail=%v, got %v", tt.expectedFail, isFail)
			}
		})
	}
}

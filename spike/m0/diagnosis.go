package main

// Diagnosis codes — stable for the duration of M0.
// These will be redesigned for production after M0 findings are reviewed.
const (
	DiagConfigMissingCredential = "CONFIG_MISSING_CREDENTIAL"
	DiagNetworkDNSError         = "NETWORK_DNS_ERROR"
	DiagNetworkTCPRefused       = "NETWORK_TCP_REFUSED"
	DiagNetworkTCPTimeout       = "NETWORK_TCP_TIMEOUT"
	DiagNetworkTCPError         = "NETWORK_TCP_ERROR"
	DiagNetworkTLSError         = "NETWORK_TLS_ERROR"
	DiagAuthenticationFailed    = "AUTHENTICATION_FAILED"
	DiagRouteNotFound           = "ROUTE_NOT_FOUND"
	DiagHealthy                 = "HEALTHY"
	DiagUnknown                 = "UNKNOWN"
)

// Correlate applies a short-circuit evidence correlation strategy to produce
// a single Diagnosis from the set of probe results.
//
// Evaluation order (lower layers are evaluated first):
//  1. Config  — is the credential present?
//  2. DNS     — can the host be resolved?
//  3. TCP     — can a connection be established?
//  4. TLS     — can a TLS handshake complete?
//  5. Provider — does the API accept the credential?
//
// If a lower layer fails, higher layers are not required (the Provider probe
// is skipped when TCP fails). This is reflected in ProbeSet: a nil pointer
// means the probe was not run.
//
// IMPORTANT: this correlation logic is a spike assumption, not a universal
// rule. Different agent runtimes may have different precedence rules.
// The production correlation engine must be designed from first principles
// after analysing the M0 findings.
func Correlate(ps ProbeSet) Diagnosis {
	// ── 1. Config: credential presence ───────────────────────────────────────
	apiKeyCV, hasKey := ps.Config["OPENAI_API_KEY"]
	if !hasKey || !apiKeyCV.Present {
		return Diagnosis{
			Code:    DiagConfigMissingCredential,
			Summary: "OPENAI_API_KEY is not set in any config source.",
			Candidates: []Candidate{{
				Code:       DiagConfigMissingCredential,
				Confidence: "high",
				Reason:     "OPENAI_API_KEY is absent from the shell environment and .env file.",
			}},
			Evidence: []string{"OPENAI_API_KEY: absent"},
			FixHint:  "Set OPENAI_API_KEY in your shell environment or .env file.",
		}
	}

	// ── 2. DNS ────────────────────────────────────────────────────────────────
	if ps.DNS != nil && ps.DNS.Status == StatusFail {
		return Diagnosis{
			Code:    DiagNetworkDNSError,
			Summary: "DNS resolution failed for the target host.",
			Candidates: []Candidate{{
				Code:       DiagNetworkDNSError,
				Confidence: "high",
				Reason:     "DNS lookup returned an error; the host cannot be reached.",
			}},
			Evidence: ps.DNS.Evidence,
			FixHint:  "Check your network connection, DNS configuration, and that the hostname is correct.",
		}
	}

	// ── 3. TCP ────────────────────────────────────────────────────────────────
	if ps.TCP != nil && ps.TCP.Status == StatusFail {
		code, summary, hint := diagnoseTCP(ps.TCP.FailureKind)
		return Diagnosis{
			Code:    code,
			Summary: summary,
			Candidates: []Candidate{{
				Code:       code,
				Confidence: "high",
				Reason:     summary,
			}},
			Evidence: ps.TCP.Evidence,
			FixHint:  hint,
		}
	}

	// ── 4. TLS ────────────────────────────────────────────────────────────────
	if ps.TLS != nil && ps.TLS.Status == StatusFail {
		return Diagnosis{
			Code:    DiagNetworkTLSError,
			Summary: "TLS handshake failed.",
			Candidates: []Candidate{{
				Code:       DiagNetworkTLSError,
				Confidence: "high",
				Reason:     "TLS handshake error; cannot establish an encrypted connection.",
			}},
			Evidence: ps.TLS.Evidence,
			FixHint:  "Verify the server's TLS certificate is valid and trusted.",
		}
	}

	// ── 5. Provider ───────────────────────────────────────────────────────────
	if ps.Provider != nil && ps.Provider.Status == StatusFail {
		return diagnoseProvider(ps.Provider)
	}

	// ── All healthy (or no probes were run) ───────────────────────────────────
	return Diagnosis{
		Code:    DiagHealthy,
		Summary: "All configured checks passed.",
		Candidates: []Candidate{{
			Code:       DiagHealthy,
			Confidence: "high",
			Reason:     "No failure detected in any probe layer.",
		}},
	}
}

func diagnoseTCP(failureKind string) (code, summary, hint string) {
	switch failureKind {
	case "tcp_refused":
		return DiagNetworkTCPRefused,
			"TCP connection was actively refused by the target.",
			"Verify the host and port are correct and the service is running."
	case "tcp_timeout":
		return DiagNetworkTCPTimeout,
			"TCP connection timed out.",
			// Do NOT claim "this means firewall" — that is not proven.
			"Possible causes: packet filtering, routing issue, unreachable host, non-responsive destination."
	default:
		return DiagNetworkTCPError,
			"TCP connection failed.",
			"Check network connectivity and target host reachability."
	}
}

func diagnoseProvider(r *ProbeResult) Diagnosis {
	switch r.FailureKind {
	case "auth_failure":
		return Diagnosis{
			Code:    DiagAuthenticationFailed,
			Summary: "Authentication failed.",
			Candidates: []Candidate{
				{
					Code:       DiagAuthenticationFailed,
					Confidence: "high",
					Reason:     "The provider rejected the credential (HTTP 401/403).",
				},
				{
					Code:       "CREDENTIAL_REVOKED_OR_SCOPED",
					Confidence: "medium",
					Reason:     "Credential may be valid but revoked, expired, or missing required scopes.",
				},
			},
			Evidence: r.Evidence,
			FixHint:  "Check that your API key is valid, not expired, and has the required permissions.",
		}
	case "route_not_found":
		return Diagnosis{
			Code:    DiagRouteNotFound,
			Summary: "The expected OpenAI-compatible /models route returned 404.",
			Candidates: []Candidate{{
				Code:       DiagRouteNotFound,
				Confidence: "medium",
				Reason:     "Route is missing; base URL may not point to an OpenAI-compatible endpoint.",
			}},
			Evidence: r.Evidence,
			FixHint:  "Verify OPENAI_BASE_URL points to an OpenAI-compatible API root (e.g., https://api.openai.com/v1).",
		}
	case "http_timeout":
		return Diagnosis{
			Code:    DiagNetworkTCPTimeout,
			Summary: "HTTP request timed out after TCP connected.",
			Candidates: []Candidate{{
				Code:       DiagNetworkTCPTimeout,
				Confidence: "medium",
				Reason:     "TCP connected but the server did not respond within the deadline.",
			}},
			Evidence: r.Evidence,
			FixHint:  "Check for proxy, firewall, or server-side processing delays.",
		}
	default:
		return Diagnosis{
			Code:    DiagUnknown,
			Summary: "Provider probe failed with an unexpected error.",
			Candidates: []Candidate{{
				Code:       DiagUnknown,
				Confidence: "low",
				Reason:     "An unclassified error occurred during the provider probe.",
			}},
			Evidence: r.Evidence,
			FixHint:  "Review the evidence above and check provider status.",
		}
	}
}

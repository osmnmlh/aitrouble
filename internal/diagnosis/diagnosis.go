package diagnosis

import (
	"strings"

	"github.com/osmnmlh/aitrouble/internal/core"
)

// Correlate takes a list of ordered probe results (from config/network/provider)
// and deterministically assigns a single Diagnosis.
func Correlate(results []core.ProbeResult) core.Diagnosis {
	for _, res := range results {
		if res.Status == core.StatusFail {
			return diagnoseFailure(res)
		}
	}

	return core.Diagnosis{
		FailingLayer: "None",
		Summary:      "No failure detected in the implemented checks.",
		FixHint:      "All tested components appear healthy.",
	}
}

func diagnoseFailure(res core.ProbeResult) core.Diagnosis {
	switch res.FailureKind {
	case "invalid_url":
		return core.Diagnosis{
			FailingLayer: "Network",
			Summary:      "The configured base URL could not be parsed into a supported target.",
			FixHint:      "Check the OPENAI_BASE_URL format in your configuration.",
		}
	case "dns_error", "dns_timeout":
		return core.Diagnosis{
			FailingLayer: "Network › DNS",
			Summary:      "DNS resolution failed before a TCP connection could be attempted.",
			FixHint:      "Check the hostname, DNS configuration, VPN, or local resolver configuration.",
		}
	case "tcp_refused":
		return core.Diagnosis{
			FailingLayer: "Network › TCP",
			Summary:      "The target actively refused the TCP connection.",
			FixHint:      "Check whether the service is listening and whether the configured host and port are correct.",
		}
	case "tcp_timeout":
		return core.Diagnosis{
			FailingLayer: "Network › TCP",
			Summary:      "The TCP connection attempt timed out.",
			FixHint:      "Check network connectivity and ensure no intermediate firewalls are dropping packets.",
		}
	case "tcp_error":
		return core.Diagnosis{
			FailingLayer: "Network › TCP",
			Summary:      "The TCP connection failed with an error.",
			FixHint:      "Check network connectivity and endpoint availability.",
		}
	case "tls_cert_error":
		return core.Diagnosis{
			FailingLayer: "Network › TLS",
			Summary:      "The TLS handshake failed due to a certificate error.",
			FixHint:      "Check endpoint certificate validity, hostname matching, or local trust configuration.",
		}
	case "tls_error", "tls_timeout":
		return core.Diagnosis{
			FailingLayer: "Network › TLS",
			Summary:      "The TLS connection failed.",
			FixHint:      "Check endpoint certificate and TLS requirements.",
		}
	case "auth_failure":
		return core.Diagnosis{
			FailingLayer: "Provider › Authentication",
			Summary:      "The network path succeeded, but the provider rejected authentication.",
			FixHint:      "Check the active OPENAI_API_KEY in your shell or .env file.",
		}
	case "route_not_found":
		return core.Diagnosis{
			FailingLayer: "Provider › /models",
			Summary:      "The configured endpoint returned HTTP 404 for the expected route.",
			FixHint:      "Verify the configured OPENAI_BASE_URL. The path /models was not found on the server.",
		}
	case "rate_limited":
		return core.Diagnosis{
			FailingLayer: "Provider",
			Summary:      "The provider returned an HTTP 429 Rate Limit error.",
			FixHint:      "Wait before retrying or check your quota.",
		}
	case "provider_server_error":
		return core.Diagnosis{
			FailingLayer: "Provider",
			Summary:      "The provider returned a 5xx server error.",
			FixHint:      "Check the provider's status page. The issue is on the server side.",
		}
	case "provider_invalid_response":
		return core.Diagnosis{
			FailingLayer: "Provider › Response",
			Summary:      "HTTP 200 was received but the expected /models response shape was not observed.",
			FixHint:      "Check if the provider genuinely supports the OpenAI-compatible /models endpoint format.",
		}
	case "http_timeout", "http_cancelled":
		return core.Diagnosis{
			FailingLayer: "Provider › HTTP",
			Summary:      "The HTTP request to the provider timed out or was cancelled.",
			FixHint:      "Check network stability or increase request timeout if the provider is slow.",
		}
	case "provider_http_error":
		return core.Diagnosis{
			FailingLayer: "Provider",
			Summary:      "The provider returned an unexpected HTTP error.",
			FixHint:      "Review the HTTP status and response evidence.",
		}
	default:
		return core.Diagnosis{
			FailingLayer: "Unknown",
			Summary:      "An unknown error occurred.",
			FixHint:      "Review the logs.",
		}
	}
}

// FormatEvidence is a helper to safely join evidence for printing.
func FormatEvidence(evidence []string) string {
	if len(evidence) == 0 {
		return ""
	}
	return strings.Join(evidence, ", ")
}

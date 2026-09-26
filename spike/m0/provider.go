package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ProviderProber probes an OpenAI-compatible /models endpoint.
//
// Security invariant: the API key stored in apiKey MUST NOT appear in any
// field of a returned ProbeResult, in any formatted output, or in any error
// string visible outside this package — even if the remote server echoes it.
type ProviderProber struct {
	client *http.Client
	apiKey string // NEVER include in Evidence, errors, or formatted output
}

// NewProviderProber creates a ProviderProber.
// transport may be nil (http.DefaultTransport is used).
// Tests pass an httptest-based transport or a custom RoundTripper.
//
// The CLI MUST NOT expose apiKey as a command-line flag.
// apiKey must come from the resolved EffectiveConfig.
func NewProviderProber(apiKey string, transport http.RoundTripper) *ProviderProber {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &ProviderProber{
		client: &http.Client{Transport: transport},
		apiKey: apiKey,
	}
}

// ProbeModels calls GET {baseURL}/models and returns a classified ProbeResult.
//
// Secret invariant (enforced by test assertNoSecret):
// After this method returns, p.apiKey MUST NOT appear in:
//   - ProbeResult.Name
//   - ProbeResult.FailureKind
//   - ProbeResult.Evidence (any element)
//   - json.Marshal(result) output
func (p *ProviderProber) ProbeModels(ctx context.Context, baseURL string) ProbeResult {
	endpoint, err := modelsEndpoint(baseURL)
	if err != nil {
		return ProbeResult{
			Name:        "Provider /models",
			Status:      StatusFail,
			FailureKind: "invalid_url",
			Evidence:    []string{"could not build /models URL from base URL"},
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ProbeResult{
			Name:        "Provider /models",
			Status:      StatusFail,
			FailureKind: "invalid_url",
			Evidence:    []string{"failed to construct HTTP request"},
		}
	}

	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	req.Header.Set("User-Agent", "aitrouble-spike-m0")
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := p.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		return ProbeResult{
			Name:        "Provider /models",
			Status:      StatusFail,
			FailureKind: classifyHTTPError(err),
			Latency:     latency,
			Evidence:    []string{safeHTTPErrorSummary(err)},
		}
	}
	defer resp.Body.Close()

	// Read body with size limit to prevent memory exhaustion.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))

	// Redact the API key from the body unconditionally — the server may echo
	// it back in an error message (tested by TestProbeModels_EchoedSecret).
	body := redactSecret(string(raw), p.apiKey)

	return p.classify(resp.StatusCode, body, latency)
}

// classify interprets the HTTP status and redacted body, returning a result
// with bounded, safe evidence. No raw response headers are included.
func (p *ProviderProber) classify(status int, body string, latency time.Duration) ProbeResult {
	switch status {
	case http.StatusOK:
		return p.handleOK(body, latency)
	case http.StatusUnauthorized, http.StatusForbidden:
		return p.handleAuthFailure(status, body, latency)
	case http.StatusNotFound:
		return ProbeResult{
			Name:        "Provider /models",
			Status:      StatusFail,
			FailureKind: "route_not_found",
			Latency:     latency,
			Evidence: []string{
				"The expected OpenAI-compatible /models route returned 404.",
				"This is evidence that the route is missing, not proof of incompatibility.",
			},
		}
	default:
		return ProbeResult{
			Name:        "Provider /models",
			Status:      StatusFail,
			FailureKind: fmt.Sprintf("http_%d", status),
			Latency:     latency,
			Evidence:    []string{fmt.Sprintf("unexpected HTTP %d", status)},
		}
	}
}

// handleOK extracts safe, bounded information from a 200 response.
// Only model count and first 3 model IDs (truncated) are included.
func (p *ProviderProber) handleOK(body string, latency time.Duration) ProbeResult {
	ev := []string{"HTTP 200"}

	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &modelsResp); err == nil {
		n := len(modelsResp.Data)
		ev = append(ev, fmt.Sprintf("model count: %d", n))
		limit := 3
		if n < limit {
			limit = n
		}
		for i := 0; i < limit; i++ {
			ev = append(ev, "model: "+truncate(modelsResp.Data[i].ID, 64))
		}
	}

	return ProbeResult{
		Name:     "Provider /models",
		Status:   StatusPass,
		Latency:  latency,
		Evidence: ev,
	}
}

// handleAuthFailure extracts safe, bounded error fields from a 401/403 body.
// Headers are never included. The raw body is never included.
func (p *ProviderProber) handleAuthFailure(status int, body string, latency time.Duration) ProbeResult {
	ev := []string{fmt.Sprintf("HTTP %d", status)}

	var errBody struct {
		Error struct {
			Code    interface{} `json:"code"` // string or null depending on provider
			Type    string      `json:"type"`
			Message string      `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &errBody); err == nil {
		if errBody.Error.Code != nil {
			ev = append(ev, "error.code: "+truncate(fmt.Sprintf("%v", errBody.Error.Code), 64))
		}
		if errBody.Error.Type != "" {
			ev = append(ev, "error.type: "+truncate(errBody.Error.Type, 64))
		}
		if errBody.Error.Message != "" {
			ev = append(ev, "error.message: "+truncate(errBody.Error.Message, 128))
		}
	}

	return ProbeResult{
		Name:        "Provider /models",
		Status:      StatusFail,
		FailureKind: "auth_failure",
		Latency:     latency,
		Evidence:    ev,
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// modelsEndpoint builds the /models URL from baseURL, normalising trailing slashes.
// It rejects patterns that would create double path segments like /v1/v1/models.
func modelsEndpoint(baseURL string) (string, error) {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return "", fmt.Errorf("empty base URL")
	}
	return base + "/models", nil
}

// redactSecret replaces all occurrences of secret in s with [REDACTED].
// Called unconditionally on all response bodies before storing in Evidence.
func redactSecret(s, secret string) string {
	if secret == "" || s == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "[REDACTED]")
}

// truncate clips s to max bytes and appends "…" if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func classifyHTTPError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "http_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "http_cancelled"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns_error"
	}
	if isConnectionRefused(err) {
		return "tcp_refused"
	}
	return "http_error"
}

func safeHTTPErrorSummary(err error) string {
	switch classifyHTTPError(err) {
	case "http_timeout":
		return "HTTP request timed out"
	case "http_cancelled":
		return "HTTP request cancelled"
	case "dns_error":
		return safeDNSSummary(err)
	case "tcp_refused":
		return "connection refused"
	default:
		return "HTTP request failed"
	}
}

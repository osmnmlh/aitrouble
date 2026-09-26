package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/osmnmlh/aitrouble/internal/core"
)

// ProviderProber probes an OpenAI-compatible /models endpoint.
type ProviderProber struct {
	client  *http.Client
	timeout time.Duration
}

// NewProviderProber creates a new ProviderProber.
// If transport is nil, http.DefaultTransport is used.
func NewProviderProber(transport http.RoundTripper, timeout time.Duration) *ProviderProber {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &ProviderProber{
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		timeout: timeout,
	}
}

// ProbeModels performs an HTTP GET /models using the provided EffectiveConfig.
func (p *ProviderProber) ProbeModels(ctx context.Context, cfg core.EffectiveConfig) core.ProbeResult {
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	baseURLVal, ok := cfg.Get("OPENAI_BASE_URL")
	if !ok {
		return p.failResult("invalid_url", "OPENAI_BASE_URL is missing")
	}
	baseURL := baseURLVal.RawValue()

	apiKeyVal, _ := cfg.Get("OPENAI_API_KEY")
	apiKey := apiKeyVal.RawValue()

	endpoint, err := buildModelsEndpoint(baseURL)
	if err != nil {
		return p.failResult("invalid_url", "could not build /models URL from base URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return p.failResult("invalid_url", "failed to construct HTTP request")
	}

	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	start := time.Now()
	resp, err := p.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		return core.ProbeResult{
			Name:        "Provider /models",
			Status:      core.StatusFail,
			FailureKind: classifyHTTPError(err),
			Latency:     latency,
			Evidence:    []string{safeHTTPErrorSummary(err)},
		}
	}
	defer resp.Body.Close()

	// 64 KB limit
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	body := redactSecret(string(raw), apiKey)

	return p.classifyResponse(resp.StatusCode, body, latency)
}

func (p *ProviderProber) classifyResponse(status int, body string, latency time.Duration) core.ProbeResult {
	switch status {
	case http.StatusOK:
		return p.handleOK(body, latency)
	case http.StatusUnauthorized, http.StatusForbidden:
		return p.handleAuthFailure(status, body, latency)
	case http.StatusNotFound:
		return core.ProbeResult{
			Name:        "Provider /models",
			Status:      core.StatusFail,
			FailureKind: "route_not_found",
			Latency:     latency,
			Evidence: []string{
				"HTTP 404",
				"expected /models route was not found",
			},
		}
	case http.StatusTooManyRequests:
		return p.failResultWithLatency("rate_limited", fmt.Sprintf("HTTP %d", status), latency)
	}

	if status >= 500 && status < 600 {
		return p.failResultWithLatency("provider_server_error", fmt.Sprintf("HTTP %d", status), latency)
	}

	return p.failResultWithLatency("provider_http_error", fmt.Sprintf("HTTP %d", status), latency)
}

func (p *ProviderProber) handleOK(body string, latency time.Duration) core.ProbeResult {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return p.invalidShapeResult(latency)
	}
	if _, ok := raw["data"]; !ok {
		return p.invalidShapeResult(latency)
	}

	// Verify data is an array
	dataArray, ok := raw["data"].([]interface{})
	if !ok {
		return p.invalidShapeResult(latency)
	}

	ev := []string{"HTTP 200", fmt.Sprintf("model count: %d", len(dataArray))}
	return core.ProbeResult{
		Name:     "Provider /models",
		Status:   core.StatusPass,
		Latency:  latency,
		Evidence: ev,
	}
}

func (p *ProviderProber) invalidShapeResult(latency time.Duration) core.ProbeResult {
	return core.ProbeResult{
		Name:        "Provider /models",
		Status:      core.StatusFail,
		FailureKind: "provider_invalid_response",
		Latency:     latency,
		Evidence: []string{
			"HTTP 200",
			"response did not contain the expected /models shape",
		},
	}
}

func (p *ProviderProber) handleAuthFailure(status int, body string, latency time.Duration) core.ProbeResult {
	ev := []string{fmt.Sprintf("HTTP %d", status)}

	var errBody struct {
		Error struct {
			Code interface{} `json:"code"`
			Type string      `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal([]byte(body), &errBody); err == nil {
		if errBody.Error.Code != nil {
			ev = append(ev, "error.code: "+truncate(fmt.Sprintf("%v", errBody.Error.Code), 64))
		}
		if errBody.Error.Type != "" {
			ev = append(ev, "error.type: "+truncate(errBody.Error.Type, 64))
		}
	}

	return core.ProbeResult{
		Name:        "Provider /models",
		Status:      core.StatusFail,
		FailureKind: "auth_failure",
		Latency:     latency,
		Evidence:    ev,
	}
}

func (p *ProviderProber) failResult(kind, evidence string) core.ProbeResult {
	return p.failResultWithLatency(kind, evidence, 0)
}

func (p *ProviderProber) failResultWithLatency(kind, evidence string, latency time.Duration) core.ProbeResult {
	return core.ProbeResult{
		Name:        "Provider /models",
		Status:      core.StatusFail,
		FailureKind: kind,
		Latency:     latency,
		Evidence:    []string{evidence},
	}
}

// -- Helpers --

func buildModelsEndpoint(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", errors.New("empty or invalid base URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/models"
	return u.String(), nil
}

func redactSecret(s, secret string) string {
	if secret == "" || s == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "[REDACTED]")
}

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

func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "actively refused") ||
		strings.Contains(s, "No connection could be made")
}

func safeHTTPErrorSummary(err error) string {
	switch classifyHTTPError(err) {
	case "http_timeout":
		return "HTTP request timed out"
	case "http_cancelled":
		return "HTTP request cancelled"
	case "dns_error":
		return "DNS lookup failed or timed out"
	case "tcp_refused":
		return "connection refused"
	default:
		return "HTTP request failed"
	}
}

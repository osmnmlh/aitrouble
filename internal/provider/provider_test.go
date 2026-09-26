package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/osmnmlh/aitrouble/internal/core"
)

// -- Helpers --

// Since core.ConfigValue has an unexported value, we can't construct EffectiveConfig directly.
// We'll use t.Setenv and core.ResolveEffectiveConfig
func setupConfig(t *testing.T, baseURL, apiKey string) core.EffectiveConfig {
	t.Setenv("OPENAI_BASE_URL", baseURL)
	if apiKey != "" {
		t.Setenv("OPENAI_API_KEY", apiKey)
	} else {
		t.Setenv("OPENAI_API_KEY", "")
	}
	cfg, err := core.ResolveEffectiveConfig("")
	if err != nil {
		t.Fatalf("failed to resolve config: %v", err)
	}
	return cfg
}

func setupProber(t *testing.T, handler http.HandlerFunc) (*ProviderProber, *httptest.Server) {
	ts := httptest.NewServer(handler)
	p := NewProviderProber(nil, 2*time.Second)
	return p, ts
}

// -- A. 200 success --
func TestProbeModels_Success(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"object": "list", "data": [{"id": "model-a"}, {"id": "model-b"}]}`))
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusPass {
		t.Errorf("expected pass, got %v: %v", res.Status, res.FailureKind)
	}
	if len(res.Evidence) < 2 || res.Evidence[1] != "model count: 2" {
		t.Errorf("unexpected evidence: %v", res.Evidence)
	}
}

// -- B. 200 invalid JSON --
func TestProbeModels_InvalidJSON(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{invalid json`))
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail || res.FailureKind != "provider_invalid_response" {
		t.Errorf("expected fail/invalid_response, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- C. 200 without data --
func TestProbeModels_NoData(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"object": "list"}`))
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail || res.FailureKind != "provider_invalid_response" {
		t.Errorf("expected fail/invalid_response, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- D. 401 --
func TestProbeModels_401(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error": {"code": "invalid_api_key", "type": "auth"}}`))
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail || res.FailureKind != "auth_failure" {
		t.Errorf("expected fail/auth_failure, got %v/%v", res.Status, res.FailureKind)
	}
	foundCode := false
	for _, e := range res.Evidence {
		if strings.Contains(e, "invalid_api_key") {
			foundCode = true
		}
	}
	if !foundCode {
		t.Errorf("expected invalid_api_key in evidence, got: %v", res.Evidence)
	}
}

// -- E. 403 --
func TestProbeModels_403(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail || res.FailureKind != "auth_failure" {
		t.Errorf("expected fail/auth_failure, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- F. 404 --
func TestProbeModels_404(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail || res.FailureKind != "route_not_found" {
		t.Errorf("expected fail/route_not_found, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- G. 429 --
func TestProbeModels_429(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail || res.FailureKind != "rate_limited" {
		t.Errorf("expected fail/rate_limited, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- H. 500 --
func TestProbeModels_500(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail || res.FailureKind != "provider_server_error" {
		t.Errorf("expected fail/provider_server_error, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- I. 418 arbitrary --
func TestProbeModels_418(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(418)
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail || res.FailureKind != "provider_http_error" {
		t.Errorf("expected fail/provider_http_error, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- J. Timeout --
func TestProbeModels_Timeout(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	})
	defer ts.Close()

	p.timeout = 10 * time.Millisecond // very short timeout
	cfg := setupConfig(t, ts.URL, "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail || res.FailureKind != "http_timeout" {
		t.Errorf("expected fail/http_timeout, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- K. Cancellation --
func TestProbeModels_Cancellation(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "test-key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	res := p.ProbeModels(ctx, cfg)

	if res.Status != core.StatusFail || res.FailureKind != "http_cancelled" {
		t.Errorf("expected fail/http_cancelled, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- L. Connection / transport error --
func TestProbeModels_TransportError(t *testing.T) {
	p := NewProviderProber(nil, time.Second)
	// Invalid port, connection refused
	cfg := setupConfig(t, "http://127.0.0.1:12345", "test-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusFail {
		t.Errorf("expected fail, got %v", res.Status)
	}
	if res.FailureKind != "tcp_refused" && res.FailureKind != "http_error" {
		t.Errorf("expected tcp_refused or http_error, got %v", res.FailureKind)
	}
}

// -- M. API key header --
func TestProbeModels_APIKeyHeader(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer the-secret-key" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{"data":[]}`))
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "the-secret-key")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusPass {
		t.Errorf("expected pass (auth header was sent), got %v", res.Status)
	}
}

// -- N. Missing API key --
func TestProbeModels_MissingAPIKey(t *testing.T) {
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("expected no auth header, got %s", auth)
		}
		w.Write([]byte(`{"data":[]}`))
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, "")
	res := p.ProbeModels(context.Background(), cfg)

	if res.Status != core.StatusPass {
		t.Errorf("expected pass, got %v", res.Status)
	}
}

// -- O. Secret echo protection --
func TestProbeModels_SecretEchoProtection(t *testing.T) {
	secret := "sk-super-secret-12345"
	p, ts := setupProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		// echo the secret back
		w.Write([]byte(`{"error": {"code": "` + secret + `", "type": "auth"}}`))
	})
	defer ts.Close()

	cfg := setupConfig(t, ts.URL, secret)
	res := p.ProbeModels(context.Background(), cfg)

	// Ensure secret is nowhere in evidence
	for _, e := range res.Evidence {
		if strings.Contains(e, secret) {
			t.Errorf("evidence leaked secret: %v", e)
		}
	}
	if strings.Contains(res.Name, secret) {
		t.Errorf("name leaked secret")
	}
	if strings.Contains(res.FailureKind, secret) {
		t.Errorf("failure kind leaked secret")
	}

	b, _ := json.Marshal(res)
	if strings.Contains(string(b), secret) {
		t.Errorf("json marshal leaked secret")
	}
}

// -- P. Base URL normalization --
func TestProbeModels_BaseURLNormalization(t *testing.T) {
	tests := []string{
		"http://example.com/v1",
		"http://example.com/v1/",
	}

	for _, tc := range tests {
		s, err := buildModelsEndpoint(tc)
		if err != nil {
			t.Errorf("failed to build: %v", err)
		}
		if s != "http://example.com/v1/models" {
			t.Errorf("expected http://example.com/v1/models, got %s for %s", s, tc)
		}
	}
}

// -- Q. No accidental double /v1 --
func TestProbeModels_DoubleV1Protection(t *testing.T) {
	// Our logic just appends /models, it doesn't automatically insert /v1
	s, _ := buildModelsEndpoint("http://example.com/v1")
	if s == "http://example.com/v1/v1/models" {
		t.Errorf("double v1 occurred")
	}
	if s != "http://example.com/v1/models" {
		t.Errorf("expected http://example.com/v1/models, got %s", s)
	}
}

// -- R. Query / fragment --
func TestProbeModels_QueryFragment(t *testing.T) {
	s, err := buildModelsEndpoint("http://example.com/v1?query=1#frag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "http://example.com/v1/models?query=1#frag" {
		t.Errorf("expected proper query/fragment handling, got %s", s)
	}
}

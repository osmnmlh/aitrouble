package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const testAPIKey = "sk-test1234567890abcdef-UNIQUE-TEST-KEY"

// ── Secret-leak invariant helpers ─────────────────────────────────────────────

// assertNoSecret verifies that the API key does NOT appear in any field of
// result. This is the core security invariant for the provider probe.
// It also marshals the result to JSON and checks the JSON string.
func assertNoSecret(t *testing.T, secret string, result ProbeResult) {
	t.Helper()
	if secret == "" {
		return
	}
	check := func(field, val string) {
		t.Helper()
		if strings.Contains(val, secret) {
			t.Errorf("SECRET LEAK in field %q: contains %q", field, secret)
		}
	}
	check("Name", result.Name)
	check("FailureKind", result.FailureKind)
	for i, e := range result.Evidence {
		check(fmt.Sprintf("Evidence[%d]", i), e)
	}
	// Also verify the JSON-marshalled form
	b, err := json.Marshal(result)
	if err == nil && strings.Contains(string(b), secret) {
		t.Errorf("SECRET LEAK in JSON-marshalled result")
	}
}

// ── Local fake server helpers ─────────────────────────────────────────────────

func serverWith(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
}

func serverWithDelay(delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// client cancelled
		}
	}))
}

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/provider/" + name)
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return string(b)
}

func newTestProber(ts *httptest.Server) *ProviderProber {
	return NewProviderProber(testAPIKey, ts.Client().Transport)
}

// ── Test: 200 OK ──────────────────────────────────────────────────────────────

func TestProbeModels_OK(t *testing.T) {
	body := loadFixture(t, "models_ok.json")
	ts := serverWith(http.StatusOK, body)
	defer ts.Close()

	p := newTestProber(ts)
	r := p.ProbeModels(context.Background(), ts.URL)

	assertStatus(t, r, StatusPass)
	assertNoSecret(t, testAPIKey, r)

	// Evidence should contain model count and model IDs
	combined := strings.Join(r.Evidence, " ")
	if !strings.Contains(combined, "model count:") {
		t.Error("expected 'model count:' in evidence")
	}
	if !strings.Contains(combined, "gpt-4o") {
		t.Error("expected model ID 'gpt-4o' in evidence")
	}
}

// ── Test: 401 Unauthorized ────────────────────────────────────────────────────

func TestProbeModels_Unauthorized(t *testing.T) {
	body := loadFixture(t, "unauthorized.json")
	ts := serverWith(http.StatusUnauthorized, body)
	defer ts.Close()

	p := newTestProber(ts)
	r := p.ProbeModels(context.Background(), ts.URL)

	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "auth_failure")
	assertNoSecret(t, testAPIKey, r)

	combined := strings.Join(r.Evidence, " ")
	if !strings.Contains(combined, "401") {
		t.Error("expected HTTP 401 in evidence")
	}
	if !strings.Contains(combined, "invalid_api_key") {
		t.Error("expected error code in evidence")
	}
}

// ── Test: 403 Forbidden ───────────────────────────────────────────────────────

func TestProbeModels_Forbidden(t *testing.T) {
	body := loadFixture(t, "forbidden.json")
	ts := serverWith(http.StatusForbidden, body)
	defer ts.Close()

	p := newTestProber(ts)
	r := p.ProbeModels(context.Background(), ts.URL)

	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "auth_failure")
	assertNoSecret(t, testAPIKey, r)
}

// ── Test: 404 Not Found ───────────────────────────────────────────────────────

func TestProbeModels_NotFound(t *testing.T) {
	body := loadFixture(t, "not_found.json")
	ts := serverWith(http.StatusNotFound, body)
	defer ts.Close()

	p := newTestProber(ts)
	r := p.ProbeModels(context.Background(), ts.URL)

	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "route_not_found")
	assertNoSecret(t, testAPIKey, r)

	// Evidence must contain the specific message, not a vague "incompatible" claim
	combined := strings.Join(r.Evidence, " ")
	if !strings.Contains(combined, "404") {
		t.Error("expected 404 in evidence")
	}
}

// ── Test: Timeout ─────────────────────────────────────────────────────────────

func TestProbeModels_Timeout(t *testing.T) {
	ts := serverWithDelay(5 * time.Second) // server responds slowly
	defer ts.Close()

	p := NewProviderProber(testAPIKey, ts.Client().Transport)
	// Very short timeout to force a deadline
	p.client.Timeout = 50 * time.Millisecond

	r := p.ProbeModels(context.Background(), ts.URL)

	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "http_timeout")
	assertNoSecret(t, testAPIKey, r)
}

// ── Test: Network error (server never responds) ────────────────────────────────

func TestProbeModels_NetworkError(t *testing.T) {
	// Create a server and immediately close it to simulate connection refused.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	url := ts.URL
	ts.Close() // close before we connect

	p := NewProviderProber(testAPIKey, nil) // uses real transport
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := p.ProbeModels(ctx, url)

	assertStatus(t, r, StatusFail)
	assertNoSecret(t, testAPIKey, r)
}

// ── Test: Malicious server echoes the API key ─────────────────────────────────

// TestProbeModels_EchoedSecret proves the secret-leak invariant holds even
// when the server intentionally includes the API key in its response body.
func TestProbeModels_EchoedSecret(t *testing.T) {
	// Build a response body that contains the test API key verbatim.
	echoBody := strings.ReplaceAll(
		loadFixture(t, "echoed_secret.json"),
		"PLACEHOLDER_KEY",
		testAPIKey,
	)
	if !strings.Contains(echoBody, testAPIKey) {
		t.Fatal("test setup error: echoBody does not contain the test key")
	}

	ts := serverWith(http.StatusUnauthorized, echoBody)
	defer ts.Close()

	p := newTestProber(ts)
	r := p.ProbeModels(context.Background(), ts.URL)

	// The result must still not contain the key.
	assertNoSecret(t, testAPIKey, r)
	assertStatus(t, r, StatusFail)
}

// ── Test: URL normalisation ───────────────────────────────────────────────────

func TestModelsEndpoint_TrailingSlash(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{"https://api.openai.com/v1", "https://api.openai.com/v1/models"},
		{"https://api.openai.com/v1/", "https://api.openai.com/v1/models"},
		{"https://api.openai.com/v1//", "https://api.openai.com/v1/models"},
	}
	for _, tc := range cases {
		t.Run(tc.base, func(t *testing.T) {
			got, err := modelsEndpoint(tc.base)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestModelsEndpoint_Empty(t *testing.T) {
	_, err := modelsEndpoint("")
	if err == nil {
		t.Error("empty base URL should return an error")
	}
}

// ── Test: redactSecret ────────────────────────────────────────────────────────

func TestRedactSecret(t *testing.T) {
	secret := "my-secret-key"
	cases := []struct {
		input string
		want  string
	}{
		{"contains my-secret-key in middle", "contains [REDACTED] in middle"},
		{"my-secret-key at start", "[REDACTED] at start"},
		{"at end my-secret-key", "at end [REDACTED]"},
		{"no secret here", "no secret here"},
		{"", ""},
	}
	for _, tc := range cases {
		got := redactSecret(tc.input, secret)
		if got != tc.want {
			t.Errorf("redactSecret(%q, secret): want %q, got %q", tc.input, tc.want, got)
		}
	}
}

func TestRedactSecret_EmptySecret(t *testing.T) {
	// Empty secret: nothing should be redacted
	got := redactSecret("some value", "")
	if got != "some value" {
		t.Errorf("empty secret: got %q", got)
	}
}

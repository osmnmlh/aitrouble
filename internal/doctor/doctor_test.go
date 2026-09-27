package doctor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/osmnmlh/aitrouble/internal/core"
)

// --- Fake probers for deterministic tests ---

type fakeNetProber struct {
	results []core.ProbeResult
}

func (f *fakeNetProber) ProbeTarget(_ context.Context, _ string) []core.ProbeResult {
	return f.results
}

type fakeProvProber struct {
	result core.ProbeResult
	called bool
}

func (f *fakeProvProber) ProbeModels(_ context.Context, _ core.EffectiveConfig) core.ProbeResult {
	f.called = true
	return f.result
}

// setBaseURL sets a minimal env so that OPENAI_BASE_URL is resolvable.
func setBaseURL(t *testing.T, url string) {
	t.Helper()
	t.Setenv("OPENAI_BASE_URL", url)
}

// --- Test A: all layers pass ---

func TestRunWithDeps_AllPass(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	prov := &fakeProvProber{result: core.ProbeResult{Name: "OpenAI /models", Status: core.StatusPass}}
	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusPass},
			{Name: "TCP", Status: core.StatusPass},
			{Name: "TLS", Status: core.StatusPass},
		}},
		prov: prov,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runWithDeps(context.Background(), "", &stdout, &stderr, d)

	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d\nstdout: %s\nstderr: %s", exitCode, stdout.String(), stderr.String())
	}
	if !prov.called {
		t.Error("expected provider to be called when network passes")
	}
	if !strings.Contains(stdout.String(), "All tested components appear healthy") {
		t.Errorf("expected healthy diagnosis, got: %s", stdout.String())
	}
}

// --- Test B: DNS failure ---

func TestRunWithDeps_DNSFailure(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://bad.example.com/v1")

	prov := &fakeProvProber{}
	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusFail, FailureKind: "dns_error", Evidence: []string{"no such host"}},
			{Name: "TCP", Status: core.StatusSkip, Evidence: []string{"skipped due to DNS failure"}},
			{Name: "TLS", Status: core.StatusSkip, Evidence: []string{"skipped due to DNS failure"}},
		}},
		prov: prov,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runWithDeps(context.Background(), "", &stdout, &stderr, d)

	if exitCode != 1 {
		t.Errorf("expected exit 1, got %d", exitCode)
	}
	if prov.called {
		t.Error("provider must NOT be called after DNS failure")
	}
	if !strings.Contains(stdout.String(), "Network › DNS") {
		t.Errorf("expected DNS diagnosis, got: %s", stdout.String())
	}
}

// --- Test C: TCP failure ---

func TestRunWithDeps_TCPFailure(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	prov := &fakeProvProber{}
	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusPass},
			{Name: "TCP", Status: core.StatusFail, FailureKind: "tcp_refused"},
			{Name: "TLS", Status: core.StatusSkip, Evidence: []string{"skipped due to TCP failure"}},
		}},
		prov: prov,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runWithDeps(context.Background(), "", &stdout, &stderr, d)

	if exitCode != 1 {
		t.Errorf("expected exit 1, got %d", exitCode)
	}
	if prov.called {
		t.Error("provider must NOT be called after TCP failure")
	}
	if !strings.Contains(stdout.String(), "Network › TCP") {
		t.Errorf("expected TCP diagnosis, got: %s", stdout.String())
	}
}

// --- Test D: TLS failure ---

func TestRunWithDeps_TLSFailure(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	prov := &fakeProvProber{}
	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusPass},
			{Name: "TCP", Status: core.StatusPass},
			{Name: "TLS", Status: core.StatusFail, FailureKind: "tls_cert_error"},
		}},
		prov: prov,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runWithDeps(context.Background(), "", &stdout, &stderr, d)

	if exitCode != 1 {
		t.Errorf("expected exit 1, got %d", exitCode)
	}
	if prov.called {
		t.Error("provider must NOT be called after TLS failure")
	}
	if !strings.Contains(stdout.String(), "Network › TLS") {
		t.Errorf("expected TLS diagnosis, got: %s", stdout.String())
	}
}

// --- Test E: provider auth failure ---

func TestRunWithDeps_AuthFailure(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	prov := &fakeProvProber{result: core.ProbeResult{
		Name:        "OpenAI /models",
		Status:      core.StatusFail,
		FailureKind: "auth_failure",
		Evidence:    []string{"HTTP 401"},
	}}
	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusPass},
			{Name: "TCP", Status: core.StatusPass},
			{Name: "TLS", Status: core.StatusPass},
		}},
		prov: prov,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runWithDeps(context.Background(), "", &stdout, &stderr, d)

	if exitCode != 1 {
		t.Errorf("expected exit 1, got %d", exitCode)
	}
	if !prov.called {
		t.Error("provider MUST be called when network passes")
	}
	if !strings.Contains(stdout.String(), "Provider › Authentication") {
		t.Errorf("expected auth diagnosis, got: %s", stdout.String())
	}
}

// --- Test F: HTTP target skips TLS ---

func TestRunWithDeps_HTTPTargetSkipsTLS(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://localhost:8080/v1")

	prov := &fakeProvProber{result: core.ProbeResult{Name: "OpenAI /models", Status: core.StatusPass}}
	// Simulate HTTP network: only DNS + TCP (no TLS)
	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusPass},
			{Name: "TCP", Status: core.StatusPass},
			// No TLS result for http:// target
		}},
		prov: prov,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runWithDeps(context.Background(), "", &stdout, &stderr, d)

	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d\nstdout: %s", exitCode, stdout.String())
	}
	if !prov.called {
		t.Error("provider MUST be called when network passes (HTTP target)")
	}
}

// --- Test G: --env-file is used ---

func TestRunWithDeps_EnvFile(t *testing.T) {
	// Create a temp .env file
	dir := t.TempDir()
	envPath := filepath.Join(dir, "test.env")
	if err := os.WriteFile(envPath, []byte("OPENAI_BASE_URL=https://custom.example.com/v1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Ensure shell env does not shadow the .env file.
	// t.Setenv("", ...) still makes LookupEnv return ok=true, so we must Unsetenv.
	os.Unsetenv("OPENAI_BASE_URL")
	os.Unsetenv("OPENAI_API_KEY")
	t.Cleanup(func() { os.Unsetenv("OPENAI_BASE_URL"); os.Unsetenv("OPENAI_API_KEY") })

	prov := &fakeProvProber{result: core.ProbeResult{Name: "OpenAI /models", Status: core.StatusPass}}
	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusPass},
			{Name: "TCP", Status: core.StatusPass},
			{Name: "TLS", Status: core.StatusPass},
		}},
		prov: prov,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runWithDeps(context.Background(), envPath, &stdout, &stderr, d)

	// Should succeed (custom URL was resolved from env file)
	if exitCode != 0 {
		t.Errorf("expected exit 0 with env-file, got %d\nstdout: %s\nstderr: %s", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "custom.example.com") {
		t.Errorf("expected custom base URL from env-file in output, got: %s", stdout.String())
	}
}

// --- Test H: secret safety in final output ---

func TestRunWithDeps_SecretNeverInOutput(t *testing.T) {
	const secret = "test-secret-key-123456"
	t.Setenv("OPENAI_API_KEY", secret)
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	prov := &fakeProvProber{result: core.ProbeResult{
		Name:        "OpenAI /models",
		Status:      core.StatusFail,
		FailureKind: "auth_failure",
		// Even if a malicious provider echoed the secret back, it should be gone here
		Evidence: []string{"HTTP 401"},
	}}
	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusPass},
			{Name: "TCP", Status: core.StatusPass},
			{Name: "TLS", Status: core.StatusPass},
		}},
		prov: prov,
	}

	var stdout, stderr bytes.Buffer
	runWithDeps(context.Background(), "", &stdout, &stderr, d)

	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, secret) {
		t.Errorf("secret leaked into output:\n%s", combined)
	}
}

// --- Regression: DNS failure must not produce provider diagnosis ---

func TestRunWithDeps_DNSFailNotProvider(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://bad.example.com/v1")

	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusFail, FailureKind: "dns_error"},
			{Name: "TCP", Status: core.StatusSkip},
			{Name: "TLS", Status: core.StatusSkip},
		}},
		prov: &fakeProvProber{},
	}

	var stdout, stderr bytes.Buffer
	runWithDeps(context.Background(), "", &stdout, &stderr, d)

	out := stdout.String()
	if strings.Contains(out, "Provider") && !strings.Contains(out, "skipped") {
		t.Errorf("DNS failure produced Provider diagnosis:\n%s", out)
	}
}

// --- Regression: TLS failure must not produce provider diagnosis ---

func TestRunWithDeps_TLSFailNotProvider(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	d := deps{
		net: &fakeNetProber{results: []core.ProbeResult{
			{Name: "DNS", Status: core.StatusPass},
			{Name: "TCP", Status: core.StatusPass},
			{Name: "TLS", Status: core.StatusFail, FailureKind: "tls_error"},
		}},
		prov: &fakeProvProber{},
	}

	var stdout, stderr bytes.Buffer
	runWithDeps(context.Background(), "", &stdout, &stderr, d)

	out := stdout.String()
	if strings.Contains(out, "Provider › Authentication") {
		t.Errorf("TLS failure produced Provider auth diagnosis:\n%s", out)
	}
	if !strings.Contains(out, "Network › TLS") {
		t.Errorf("expected TLS layer diagnosis, got:\n%s", out)
	}
}

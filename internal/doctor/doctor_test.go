package doctor

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// We won't test external networks, but we will test that Doctor returns expected output
// when env variables are missing or malformed.
func TestRun_MissingBaseURL(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// With no env variables set, OPENAI_BASE_URL defaults to api.openai.com/v1.
	// We'll set OPENAI_BASE_URL to empty to trigger the error.
	t.Setenv("OPENAI_BASE_URL", " ")

	exitCode := Run(context.Background(), "", &stdout, &stderr)
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "Missing OPENAI_BASE_URL") && !strings.Contains(stderr.String(), "Failed to resolve configuration") && !strings.Contains(stdout.String(), "(not set)") {
		t.Errorf("expected missing url error, got stderr: %s, stdout: %s", stderr.String(), stdout.String())
	}
}

func TestRun_NetworkFailureSkipsProvider(t *testing.T) {
	// A bad URL triggers a network parse failure, which causes network fail,
	// and provider should be skipped.
	t.Setenv("OPENAI_BASE_URL", "http://[::1]:namedport") // Invalid URL for net/url or network parse

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), "", &stdout, &stderr)

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	out := stdout.String()
	if !strings.Contains(out, "skipped due to network failure") {
		t.Errorf("expected provider to be skipped due to network failure, got: %s", out)
	}
}

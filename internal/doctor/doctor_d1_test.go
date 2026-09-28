package doctor

import (
	"bytes"
	"context"
	"testing"
)

func TestDoctor_EmptyBaseURL(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "")
	
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), "", &stdout, &stderr)

	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Missing OPENAI_BASE_URL")) {
		t.Errorf("expected Missing OPENAI_BASE_URL in stderr, got:\nSTDOUT:\n%s\nSTDERR:\n%s", stdout.String(), stderr.String())
	}
}

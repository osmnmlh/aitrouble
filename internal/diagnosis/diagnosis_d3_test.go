package diagnosis

import (
	"testing"
	"github.com/osmnmlh/aitrouble/internal/core"
)

func TestDiagnoseFailure_D3(t *testing.T) {
	res := core.ProbeResult{
		Status:      core.StatusFail,
		FailureKind: "http_error",
	}
	diag := diagnoseFailure(res)
	if diag.FailingLayer != "Provider › HTTP" {
		t.Errorf("expected Provider › HTTP, got %s", diag.FailingLayer)
	}
}

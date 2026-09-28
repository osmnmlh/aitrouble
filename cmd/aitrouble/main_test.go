package main

import (
	"bytes"
	"strings"
	"testing"
)

// --- CLI regression tests (unit-level, no subprocess) ---

func TestCLI_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("--version: expected exit 0, got %d", code)
	}
	out := stdout.String()
	if !strings.HasPrefix(out, "aitrouble ") {
		t.Errorf("--version: expected 'aitrouble <version>', got: %q", out)
	}
}

func TestCLI_VersionShortFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-v"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("-v: expected exit 0, got %d", code)
	}
}

func TestCLI_DoctorHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("doctor --help: expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
}

func TestCLI_TopLevelHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("--help: expected exit 0, got %d", code)
	}
	if stdout.String() == "" {
		t.Error("--help: expected usage text on stdout")
	}
}

func TestCLI_TopLevelHelpShort(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-h"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("-h: expected exit 0, got %d", code)
	}
	if stdout.String() == "" {
		t.Error("-h: expected usage text on stdout")
	}
}

func TestCLI_NoArgs(t *testing.T) {
	// No args → usage + exit 2
	var stdout, stderr bytes.Buffer
	code := run([]string{}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("no args: expected exit 2, got %d", code)
	}
	if stdout.String() == "" {
		t.Error("no args: expected usage text on stdout")
	}
}

func TestCLI_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"notacommand"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("unknown command: expected exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("unknown command: expected error message in stderr, got: %q", stderr.String())
	}
}

func TestCLI_VersionFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run([]string{"--version"}, &stdout, &stderr)
	out := strings.TrimSpace(stdout.String())
	// Must be "aitrouble <something>" with no extra lines
	if strings.Contains(out, "\n") {
		t.Errorf("--version: output should be single line, got: %q", out)
	}
	parts := strings.SplitN(out, " ", 2)
	if len(parts) != 2 || parts[0] != "aitrouble" {
		t.Errorf("--version: unexpected format: %q", out)
	}
}

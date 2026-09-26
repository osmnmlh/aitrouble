package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	bin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not in PATH")
	}

	out, err := exec.Command(bin, "run", ".", "--version").Output()
	if err != nil {
		t.Fatalf("run --version: %v", err)
	}

	got := strings.TrimSpace(string(out))
	if !strings.HasPrefix(got, "aitrouble ") {
		t.Errorf("unexpected version output: %q", got)
	}
}

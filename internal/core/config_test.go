package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A: Sadece shell
func TestConfig_ShellOnly(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "shell-key")
	defer os.Unsetenv("OPENAI_API_KEY")

	cfg, err := ResolveEffectiveConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := cfg.Get("OPENAI_API_KEY")
	if !ok || !val.Present || val.Source != "shell" || val.RawValue() != "shell-key" {
		t.Errorf("expected shell-key from shell, got: %+v", val)
	}
}

// B: Sadece .env
func TestConfig_DotEnvOnly(t *testing.T) {
	os.Unsetenv("OPENAI_API_KEY")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("OPENAI_API_KEY=dotenv-key\n"), 0644)

	cfg, err := ResolveEffectiveConfig(envPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := cfg.Get("OPENAI_API_KEY")
	if !ok || !val.Present || val.Source != ".env" || val.RawValue() != "dotenv-key" {
		t.Errorf("expected dotenv-key from .env, got: %+v", val)
	}
}

// C: İkisi birden
func TestConfig_ShellWinsOverDotEnv(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "shell-wins")
	defer os.Unsetenv("OPENAI_API_KEY")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("OPENAI_API_KEY=dotenv-loses\n"), 0644)

	cfg, err := ResolveEffectiveConfig(envPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := cfg.Get("OPENAI_API_KEY")
	if !ok || !val.Present || val.Source != "shell" || val.RawValue() != "shell-wins" {
		t.Errorf("expected shell to win, got: %+v", val)
	}
}

// D: Hiçbiri
func TestConfig_Neither(t *testing.T) {
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_BASE_URL")

	cfg, err := ResolveEffectiveConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	apiKey, ok := cfg.Get("OPENAI_API_KEY")
	if !ok || apiKey.Present || apiKey.Source != "absent" {
		t.Errorf("expected API key to be absent, got: %+v", apiKey)
	}

	baseURL, ok := cfg.Get("OPENAI_BASE_URL")
	if !ok || !baseURL.Present || baseURL.Source != "default" || baseURL.RawValue() != "https://api.openai.com/v1" {
		t.Errorf("expected base URL to be default, got: %+v", baseURL)
	}
}

// E: Shell empty
func TestConfig_ShellEmpty(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "")
	defer os.Unsetenv("OPENAI_API_KEY")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("OPENAI_API_KEY=dotenv-ignored\n"), 0644)

	cfg, err := ResolveEffectiveConfig(envPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := cfg.Get("OPENAI_API_KEY")
	if !ok || !val.Present || val.Source != "shell" || val.RawValue() != "" {
		t.Errorf("expected empty value from shell, got: %+v", val)
	}
}

// F: Secret display
func TestConfig_SecretDisplay(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "super-secret-key")
	defer os.Unsetenv("OPENAI_API_KEY")
	os.Setenv("OPENAI_BASE_URL", "https://custom.com/v1")
	defer os.Unsetenv("OPENAI_BASE_URL")

	cfg, _ := ResolveEffectiveConfig("")

	apiKey, _ := cfg.Get("OPENAI_API_KEY")
	if str := apiKey.String(); str != "[REDACTED]" {
		t.Errorf("expected API key to be redacted, got: %s", str)
	}
	if str := fmt.Sprintf("%s", apiKey); str != "[REDACTED]" {
		t.Errorf("expected format string to use String() and redact, got: %s", str)
	}

	baseURL, _ := cfg.Get("OPENAI_BASE_URL")
	if str := baseURL.String(); str != "https://custom.com/v1" {
		t.Errorf("expected base URL to be displayed, got: %s", str)
	}

	// Absent secret
	os.Unsetenv("OPENAI_API_KEY")
	cfg2, _ := ResolveEffectiveConfig("")
	apiKeyAbsent, _ := cfg2.Get("OPENAI_API_KEY")
	if str := apiKeyAbsent.String(); str != "absent" {
		t.Errorf("expected absent API key to be displayed as 'absent', got: %s", str)
	}

	// Empty non-secret
	os.Setenv("OPENAI_BASE_URL", "")
	cfg3, _ := ResolveEffectiveConfig("")
	emptyURL, _ := cfg3.Get("OPENAI_BASE_URL")
	if str := emptyURL.String(); str != `""` {
		t.Errorf("expected empty URL to be displayed as '\"\"', got: %s", str)
	}
}

// G: Missing .env
func TestConfig_MissingDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "does_not_exist.env")

	_, err := ResolveEffectiveConfig(envPath)
	if err != nil {
		t.Errorf("missing .env should not return error, got: %v", err)
	}
}

// H: Invalid filesystem error
func TestConfig_InvalidFilesystemError(t *testing.T) {
	dir := t.TempDir()

	// Attempting to read a directory as a file is an error on all platforms
	_, err := ResolveEffectiveConfig(dir)
	if err == nil {
		t.Errorf("expected error for directory, got nil")
	}
}

// I: Quoted .env
func TestConfig_QuotedDotEnv(t *testing.T) {
	os.Unsetenv("OPENAI_BASE_URL")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := `
OPENAI_BASE_URL="https://example.com/v1"
`
	os.WriteFile(envPath, []byte(content), 0644)

	cfg, _ := ResolveEffectiveConfig(envPath)
	val, ok := cfg.Get("OPENAI_BASE_URL")

	if !ok || !val.Present || val.Source != ".env" || val.RawValue() != "https://example.com/v1" {
		t.Errorf("expected unquoted value, got: %+v", val)
	}
}

// J: CRLF / BOM
func TestConfig_CRLFandBOM(t *testing.T) {
	os.Unsetenv("OPENAI_BASE_URL")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	// UTF-8 BOM + CRLF endings
	content := []byte{0xEF, 0xBB, 0xBF}
	content = append(content, []byte("OPENAI_BASE_URL=https://bom.com/v1\r\n")...)
	os.WriteFile(envPath, content, 0644)

	cfg, _ := ResolveEffectiveConfig(envPath)
	val, ok := cfg.Get("OPENAI_BASE_URL")

	if !ok || !val.Present || val.Source != ".env" || val.RawValue() != "https://bom.com/v1" {
		t.Errorf("expected parsed value despite BOM/CRLF, got: %+v", val)
	}
}

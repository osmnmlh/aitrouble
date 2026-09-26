package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── ParseDotEnv unit tests ────────────────────────────────────────────────────

func TestParseDotEnv_Basic(t *testing.T) {
	f := openFixture(t, "config/basic.env")
	defer f.Close()

	result, err := ParseDotEnv(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "OPENAI_API_KEY", result["OPENAI_API_KEY"], "sk-basic123")
	assertEqual(t, "OPENAI_BASE_URL", result["OPENAI_BASE_URL"], "https://api.example.com/v1")
}

func TestParseDotEnv_QuotedValues(t *testing.T) {
	f := openFixture(t, "config/quoted.env")
	defer f.Close()

	result, err := ParseDotEnv(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// TC-10: quoted value
	assertEqual(t, "OPENAI_API_KEY double-quoted", result["OPENAI_API_KEY"], "sk-quoted-double")
	assertEqual(t, "OPENAI_BASE_URL single-quoted", result["OPENAI_BASE_URL"], "https://quoted-single.example.com/v1")

	// TC-11: export KEY=value
	assertEqual(t, "EXPORTED_KEY", result["EXPORTED_KEY"], "exported-value")

	// Inline comment is stripped from unquoted value
	assertEqual(t, "INLINE_COMMENT", result["INLINE_COMMENT"], "value-only")
}

func TestParseDotEnv_CRLF(t *testing.T) {
	// TC-8: CRLF .env
	f := openFixture(t, "config/crlf.env")
	defer f.Close()

	result, err := ParseDotEnv(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "OPENAI_API_KEY from CRLF file", result["OPENAI_API_KEY"], "sk-crlf")
	assertEqual(t, "OPENAI_BASE_URL from CRLF file", result["OPENAI_BASE_URL"], "https://crlf.example.com/v1")
	// TC-6 via fixture: EMPTY_VAL= (present but empty)
	if val, ok := result["EMPTY_VAL"]; !ok {
		t.Error("EMPTY_VAL should be present in CRLF fixture")
	} else if val != "" {
		t.Errorf("EMPTY_VAL should be empty string, got %q", val)
	}
}

func TestParseDotEnv_UTF8BOM(t *testing.T) {
	// TC-9: UTF-8 BOM at start of file
	f := openFixture(t, "config/bom.env")
	defer f.Close()

	result, err := ParseDotEnv(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "OPENAI_API_KEY from BOM file", result["OPENAI_API_KEY"], "sk-bom")
}

func TestParseDotEnv_CommentsAndWhitespace(t *testing.T) {
	// TC-12: comments and whitespace
	input := `
# This is a full-line comment
  # Indented comment

KEY_A=value_a
KEY_B = value_b_with_spaces_around_eq

# Another comment
KEY_C=value_c
`
	result, err := ParseDotEnv(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "KEY_A", result["KEY_A"], "value_a")
	// Note: spaces around = are stripped from key but NOT from value (value_b_with_spaces_around_eq)
	assertEqual(t, "KEY_B", result["KEY_B"], "value_b_with_spaces_around_eq")
	assertEqual(t, "KEY_C", result["KEY_C"], "value_c")
	if _, ok := result["#"]; ok {
		t.Error("comment should not be parsed as a key")
	}
}

func TestParseDotEnv_ExportPrefix(t *testing.T) {
	// TC-11: export KEY=value
	input := "export EXPORTED=hello\nexport\tTAB_EXPORT=world\n"
	result, err := ParseDotEnv(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "EXPORTED", result["EXPORTED"], "hello")
	assertEqual(t, "TAB_EXPORT", result["TAB_EXPORT"], "world")
}

// ── ResolveEffectiveConfig tests ──────────────────────────────────────────────

// TC-1: shell only
func TestResolveEffectiveConfig_ShellOnly(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-shell-only")
	t.Setenv("OPENAI_BASE_URL", "https://shell.example.com/v1")

	cfg, err := ResolveEffectiveConfig(ConfigKeys, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCV(t, cfg["OPENAI_API_KEY"], true, "shell", true)
	assertCV(t, cfg["OPENAI_BASE_URL"], true, "shell", false)
	assertEqual(t, "base URL raw value", cfg["OPENAI_BASE_URL"].RawValue(), "https://shell.example.com/v1")
}

// TC-2: .env only
func TestResolveEffectiveConfig_DotEnvOnly(t *testing.T) {
	// Unset shell env so only .env is the source.
	// t.Setenv with an explicit unset-then-restore is the safe pattern.
	unsetEnvForTest(t, "OPENAI_API_KEY")
	unsetEnvForTest(t, "OPENAI_BASE_URL")

	envFile := writeTestEnv(t, "OPENAI_API_KEY=sk-dotenv\nOPENAI_BASE_URL=https://dotenv.example.com/v1\n")

	cfg, err := ResolveEffectiveConfig(ConfigKeys, envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCV(t, cfg["OPENAI_API_KEY"], true, ".env", true)
	assertCV(t, cfg["OPENAI_BASE_URL"], true, ".env", false)
}

// TC-3: shell + .env → shell wins
func TestResolveEffectiveConfig_ShellWinsOverDotEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-from-shell")
	t.Setenv("OPENAI_BASE_URL", "https://from-shell.example.com/v1")

	envFile := writeTestEnv(t, "OPENAI_API_KEY=sk-from-dotenv\nOPENAI_BASE_URL=https://from-dotenv.example.com/v1\n")

	cfg, err := ResolveEffectiveConfig(ConfigKeys, envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Shell must win.
	assertCV(t, cfg["OPENAI_API_KEY"], true, "shell", true)
	assertCV(t, cfg["OPENAI_BASE_URL"], true, "shell", false)
	assertEqual(t, "shell wins: base URL", cfg["OPENAI_BASE_URL"].RawValue(), "https://from-shell.example.com/v1")
}

// TC-4: neither → absent
func TestResolveEffectiveConfig_Neither(t *testing.T) {
	unsetEnvForTest(t, "OPENAI_API_KEY")
	unsetEnvForTest(t, "OPENAI_BASE_URL")

	cfg, err := ResolveEffectiveConfig(ConfigKeys, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCV(t, cfg["OPENAI_API_KEY"], false, "absent", true)
	assertCV(t, cfg["OPENAI_BASE_URL"], false, "absent", false)
}

// TC-5: shell variable present but empty
func TestResolveEffectiveConfig_ShellEmptyValue(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "") // set to empty — Present=true but rawValue is ""
	unsetEnvForTest(t, "OPENAI_BASE_URL")

	cfg, err := ResolveEffectiveConfig(ConfigKeys, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cv := cfg["OPENAI_API_KEY"]
	if !cv.Present {
		t.Error("key should be Present=true even when value is empty")
	}
	if cv.Source != "shell" {
		t.Errorf("source should be shell, got %q", cv.Source)
	}
	if cv.RawValue() != "" {
		t.Errorf("raw value should be empty string, got %q", cv.RawValue())
	}
}

// TC-6: .env variable present but empty
func TestResolveEffectiveConfig_DotEnvEmptyValue(t *testing.T) {
	unsetEnvForTest(t, "OPENAI_API_KEY")
	envFile := writeTestEnv(t, "OPENAI_API_KEY=\n")

	cfg, err := ResolveEffectiveConfig(ConfigKeys, envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cv := cfg["OPENAI_API_KEY"]
	if !cv.Present {
		t.Error("key should be Present=true even when value is empty in .env")
	}
	if cv.Source != ".env" {
		t.Errorf("source should be .env, got %q", cv.Source)
	}
	if cv.RawValue() != "" {
		t.Errorf("raw value should be empty string, got %q", cv.RawValue())
	}
}

// TC-7: missing .env → graceful (no error)
func TestResolveEffectiveConfig_MissingDotEnv(t *testing.T) {
	unsetEnvForTest(t, "OPENAI_API_KEY")
	unsetEnvForTest(t, "OPENAI_BASE_URL")

	cfg, err := ResolveEffectiveConfig(ConfigKeys, "/nonexistent/path/.env")
	if err != nil {
		t.Fatalf("missing .env should not return an error, got: %v", err)
	}

	assertCV(t, cfg["OPENAI_API_KEY"], false, "absent", true)
}

// ── DisplayValue / secret redaction tests ─────────────────────────────────────

func TestConfigValue_DisplayValue_SecretNeverRevealed(t *testing.T) {
	cv := ConfigValue{
		Key:      "OPENAI_API_KEY",
		Present:  true,
		Source:   "shell",
		Secret:   true,
		rawValue: "sk-super-secret-value-1234",
	}

	display := cv.DisplayValue()
	if strings.Contains(display, "sk-super-secret-value-1234") {
		t.Error("secret value leaked in DisplayValue()")
	}
	if display != "[REDACTED]" {
		t.Errorf("expected [REDACTED], got %q", display)
	}
	// Must not use sk-****1234 style
	if strings.Contains(display, "****") || strings.Contains(display, "sk-") {
		t.Error("must not use suffix-revealing redaction style")
	}
}

func TestConfigValue_DisplayValue_NonSecret(t *testing.T) {
	cv := ConfigValue{
		Key:      "OPENAI_BASE_URL",
		Present:  true,
		Source:   ".env",
		Secret:   false,
		rawValue: "https://example.com/v1",
	}
	if got := cv.DisplayValue(); got != "https://example.com/v1" {
		t.Errorf("non-secret: expected raw value, got %q", got)
	}
}

func TestConfigValue_DisplayValue_Absent(t *testing.T) {
	cv := ConfigValue{Key: "OPENAI_API_KEY", Present: false, Secret: true}
	if got := cv.DisplayValue(); got != "absent" {
		t.Errorf("absent: expected 'absent', got %q", got)
	}
}

func TestConfigValue_DisplayValue_EmptyNonSecret(t *testing.T) {
	cv := ConfigValue{Key: "OPENAI_BASE_URL", Present: true, Source: ".env", rawValue: ""}
	if got := cv.DisplayValue(); got != "(empty)" {
		t.Errorf("empty non-secret: expected '(empty)', got %q", got)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// unsetEnvForTest unsets an env var and restores it on test cleanup.
// Safe to call even if the var was not previously set.
func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	prev, waSet := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if waSet {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

func openFixture(t *testing.T, name string) *os.File {
	t.Helper()
	path := filepath.Join("testdata", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture %s: %v", path, err)
	}
	return f
}

// writeTestEnv writes content to a temp file and returns its path.
func writeTestEnv(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.env")
	if err != nil {
		t.Fatalf("create temp env file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp env file: %v", err)
	}
	f.Close()
	return f.Name()
}

func assertCV(t *testing.T, cv ConfigValue, wantPresent bool, wantSource string, wantSecret bool) {
	t.Helper()
	if cv.Present != wantPresent {
		t.Errorf("[%s] Present: want %v, got %v", cv.Key, wantPresent, cv.Present)
	}
	if cv.Source != wantSource {
		t.Errorf("[%s] Source: want %q, got %q", cv.Key, wantSource, cv.Source)
	}
	if cv.Secret != wantSecret {
		t.Errorf("[%s] Secret: want %v, got %v", cv.Key, wantSecret, cv.Secret)
	}
}

func assertEqual(t *testing.T, label, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: want %q, got %q", label, want, got)
	}
}

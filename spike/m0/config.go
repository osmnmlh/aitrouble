package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"
	"unicode"
)

// ConfigKeys are the configuration keys this spike resolves.
// Shell env and .env file are both checked for each key.
var ConfigKeys = []string{
	"OPENAI_API_KEY",
	"OPENAI_BASE_URL",
}

// secretKeys maps upper-case key names that must never be displayed.
var secretKeys = map[string]bool{
	"OPENAI_API_KEY":    true,
	"ANTHROPIC_API_KEY": true,
	"GEMINI_API_KEY":    true,
}

// isSecretKey returns true when the key's value must never appear in output.
func isSecretKey(key string) bool {
	return secretKeys[strings.ToUpper(key)]
}

// ResolveEffectiveConfig resolves each key in keys using the precedence:
//
//	shell env  >  .env file  >  absent
//
// If envFile is "" or the file does not exist, the .env layer is silently
// skipped — no error is returned for a missing file.
//
// Note: this is the M0 spike policy. Production precedence may differ for
// different agent runtimes and MUST NOT be assumed from this implementation.
func ResolveEffectiveConfig(keys []string, envFile string) (map[string]ConfigValue, error) {
	dotenv, err := loadDotEnv(envFile)
	if err != nil {
		return nil, err
	}

	result := make(map[string]ConfigValue, len(keys))
	for _, k := range keys {
		result[k] = resolveKey(k, dotenv)
	}
	return result, nil
}

// loadDotEnv loads a .env file, returning an empty map when the file is
// missing or envFile is "".
func loadDotEnv(envFile string) (map[string]string, error) {
	if envFile == "" {
		return map[string]string{}, nil
	}
	f, err := os.Open(envFile)
	if err != nil {
		// Graceful skip: missing .env is not an error.
		return map[string]string{}, nil //nolint:nilerr
	}
	defer f.Close()
	return ParseDotEnv(f)
}

// resolveKey resolves a single key according to the effective config precedence.
func resolveKey(key string, dotenv map[string]string) ConfigValue {
	secret := isSecretKey(key)

	// Shell environment has highest precedence.
	if shellVal, ok := os.LookupEnv(key); ok {
		return ConfigValue{
			Key:      key,
			Present:  true,
			Source:   "shell",
			Secret:   secret,
			rawValue: shellVal,
		}
	}

	// .env file is checked next.
	if dotVal, ok := dotenv[key]; ok {
		return ConfigValue{
			Key:      key,
			Present:  true,
			Source:   ".env",
			Secret:   secret,
			rawValue: dotVal,
		}
	}

	// Not found in any source.
	return ConfigValue{
		Key:     key,
		Present: false,
		Source:  "absent",
		Secret:  secret,
	}
}

// ParseDotEnv parses a .env file from r.
//
// Supported subset (documented here as the M0 spike contract):
//
//   - KEY=value
//   - KEY="double quoted value"
//   - KEY='single quoted value'
//   - export KEY=value  (export prefix is stripped)
//   - # comment lines  (skipped)
//   - blank lines      (skipped)
//   - CRLF line endings (normalised to LF)
//   - UTF-8 BOM at start of file (stripped)
//   - inline comment after unquoted value: KEY=value # comment
//
// NOT supported: multiline values, variable interpolation, escaped quotes,
// nested quotes, shell-style special characters.
func ParseDotEnv(r io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Strip UTF-8 BOM (EF BB BF).
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	// Normalise line endings: CRLF → LF, standalone CR → LF.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))

	result := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		parseDotEnvLine(sc.Text(), result)
	}
	return result, sc.Err()
}

// parseDotEnvLine parses one line and writes the key/value pair to out.
// Invalid or comment lines are silently skipped.
func parseDotEnvLine(line string, out map[string]string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}

	// Strip optional "export" prefix (with space or tab after it).
	if after, ok := strings.CutPrefix(line, "export "); ok {
		line = strings.TrimSpace(after)
	} else if after, ok := strings.CutPrefix(line, "export\t"); ok {
		line = strings.TrimSpace(after)
	}

	// Require at least one character before '='.
	eqIdx := strings.IndexByte(line, '=')
	if eqIdx <= 0 {
		return
	}

	key := strings.TrimSpace(line[:eqIdx])
	if !isValidEnvKey(key) {
		return
	}

	val := line[eqIdx+1:]

	// Strip inline comment only when the value is not quoted.
	val = stripInlineComment(val)

	// Trim surrounding whitespace from unquoted values.
	// Quoted values preserve internal whitespace (handled via unquoteEnvValue).
	if len(val) == 0 || (val[0] != '"' && val[0] != '\'') {
		val = strings.TrimSpace(val)
	}

	// Strip matching surrounding quotes.
	val = unquoteEnvValue(val)

	out[key] = val
}

// isValidEnvKey returns true when key looks like a shell variable name:
// starts with a letter or underscore, followed by letters, digits, underscores.
func isValidEnvKey(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
		} else {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				return false
			}
		}
	}
	return true
}

// unquoteEnvValue strips a matching pair of surrounding quotes from s.
func unquoteEnvValue(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// stripInlineComment removes a trailing # comment from an unquoted value.
// If the value starts with a quote character, it is returned unchanged
// (comment stripping inside quoted values is not supported).
func stripInlineComment(s string) string {
	if s == "" {
		return s
	}
	if s[0] == '"' || s[0] == '\'' {
		return s // quoted: skip comment stripping
	}
	if idx := strings.IndexByte(s, '#'); idx >= 0 {
		s = strings.TrimRight(s[:idx], " \t")
	}
	return s
}

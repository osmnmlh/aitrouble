package core

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

// ConfigValue represents a resolved configuration value.
type ConfigValue struct {
	Key     string
	Source  string
	Secret  bool
	Present bool
	value   string
}

// RawValue returns the unredacted configuration value.
func (v ConfigValue) RawValue() string {
	return v.value
}

// IsEmpty returns true if the value is missing or an empty string.
func (v ConfigValue) IsEmpty() bool {
	return !v.Present || v.value == ""
}

// String safely formats the configuration value, redacting secrets.
func (v ConfigValue) String() string {
	if !v.Present {
		return "absent"
	}
	if v.Secret {
		return "[REDACTED]"
	}
	if v.value == "" {
		return `""` // explicitly show empty string for non-secrets
	}
	return v.value
}

// GoString ensures %#v formatting safely redacts secrets.
func (v ConfigValue) GoString() string {
	return v.String()
}

// EffectiveConfig holds the resolved configuration for the application.
type EffectiveConfig struct {
	Values map[string]ConfigValue
}

// Get retrieves a ConfigValue by key.
func (c EffectiveConfig) Get(key string) (ConfigValue, bool) {
	v, ok := c.Values[key]
	return v, ok
}

// isSecretKey defines which configuration keys are secrets.
func isSecretKey(key string) bool {
	return key == "OPENAI_API_KEY"
}

// defaultValues defines default values for specific keys.
var defaultValues = map[string]string{
	"OPENAI_BASE_URL": "https://api.openai.com/v1",
}

// supportedKeys defines the keys supported by aitrouble v0.1.
var supportedKeys = []string{
	"OPENAI_API_KEY",
	"OPENAI_BASE_URL",
}

// ResolveEffectiveConfig resolves configuration using the precedence:
// shell environment > .env file > default
func ResolveEffectiveConfig(envFile string) (EffectiveConfig, error) {
	if envFile == "" {
		envFile = ".env"
	}
	dotenv, err := loadDotEnv(envFile)
	if err != nil {
		return EffectiveConfig{}, err
	}

	result := make(map[string]ConfigValue, len(supportedKeys))
	for _, key := range supportedKeys {
		result[key] = resolveKey(key, dotenv)
	}

	return EffectiveConfig{Values: result}, nil
}

func loadDotEnv(envFile string) (map[string]string, error) {
	f, err := os.Open(envFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Graceful skip: missing .env is not an error.
			return map[string]string{}, nil
		}
		// Propagate actual filesystem errors.
		return nil, fmt.Errorf("failed to open .env file: %w", err)
	}
	defer f.Close()
	return ParseDotEnv(f)
}

func resolveKey(key string, dotenv map[string]string) ConfigValue {
	secret := isSecretKey(key)

	// 1. Shell environment
	if shellVal, ok := os.LookupEnv(key); ok {
		return ConfigValue{
			Key:     key,
			Present: true,
			Source:  "shell",
			Secret:  secret,
			value:   shellVal,
		}
	}

	// 2. .env file
	if dotVal, ok := dotenv[key]; ok {
		return ConfigValue{
			Key:     key,
			Present: true,
			Source:  ".env",
			Secret:  secret,
			value:   dotVal,
		}
	}

	// 3. Default
	if defVal, ok := defaultValues[key]; ok {
		return ConfigValue{
			Key:     key,
			Present: true,
			Source:  "default",
			Secret:  secret,
			value:   defVal,
		}
	}

	// Absent
	return ConfigValue{
		Key:     key,
		Present: false,
		Source:  "absent",
		Secret:  secret,
	}
}

// ParseDotEnv parses a .env file from r.
func ParseDotEnv(r io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Strip UTF-8 BOM
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	// Normalise line endings
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))

	result := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		parseDotEnvLine(sc.Text(), result)
	}
	return result, sc.Err()
}

func parseDotEnvLine(line string, out map[string]string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}

	// Strip export prefix
	if after, ok := strings.CutPrefix(line, "export "); ok {
		line = strings.TrimSpace(after)
	} else if after, ok := strings.CutPrefix(line, "export\t"); ok {
		line = strings.TrimSpace(after)
	}

	eqIdx := strings.IndexByte(line, '=')
	if eqIdx <= 0 {
		return
	}

	key := strings.TrimSpace(line[:eqIdx])
	if !isValidEnvKey(key) {
		return
	}

	val := line[eqIdx+1:]
	val = stripInlineComment(val)

	if len(val) == 0 || (val[0] != '"' && val[0] != '\'') {
		val = strings.TrimSpace(val)
	}

	val = unquoteEnvValue(val)
	out[key] = val
}

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

func unquoteEnvValue(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func stripInlineComment(s string) string {
	inSQuote := false
	inDQuote := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && !inDQuote {
			inSQuote = !inSQuote
		} else if c == '"' && !inSQuote {
			inDQuote = !inDQuote
		} else if c == '#' && !inSQuote && !inDQuote {
			if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
				return strings.TrimRight(s[:i], " \t")
			}
		}
	}
	return s
}

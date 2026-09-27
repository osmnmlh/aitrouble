// Package mcp implements local MCP configuration discovery and static validation.
// It does NOT execute discovered commands or make network connections.
package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Transport identifies the MCP server transport type.
type Transport string

const (
	TransportStdio   Transport = "stdio"
	TransportHTTP    Transport = "http"
	TransportUnknown Transport = "unknown"
)

// ServerConfig holds the statically-parsed configuration of one MCP server.
// M5A never executes Command or Args.
type ServerConfig struct {
	Name      string
	Transport Transport
	Command   string // for stdio servers; never executed
	ArgCount  int    // number of args — raw args never printed
	EnvCount  int    // number of env vars — values never printed
	SafeURL   string // for http servers — credentials/tokens already redacted
}

// ConfigSource represents a discovered MCP configuration file.
type ConfigSource struct {
	Name    string
	Path    string
	Status  SourceStatus
	Detail  string
	Servers []ServerConfig
}

// SourceStatus represents the parse/discovery status of a config source.
type SourceStatus string

const (
	SourceStatusFound    SourceStatus = "found"
	SourceStatusInvalid  SourceStatus = "invalid"
	SourceStatusNotFound SourceStatus = "not_found"
)

// DiscoveryResult holds all discovered MCP configuration sources.
type DiscoveryResult struct {
	Sources []ConfigSource
}

// HasAny returns true if at least one configuration source was found (valid or invalid).
func (r DiscoveryResult) HasAny() bool {
	for _, s := range r.Sources {
		if s.Status != SourceStatusNotFound {
			return true
		}
	}
	return false
}

// HasFailure returns true if any found config source failed to parse.
func (r DiscoveryResult) HasFailure() bool {
	for _, s := range r.Sources {
		if s.Status == SourceStatusInvalid {
			return true
		}
	}
	return false
}

// TotalServers returns the total number of successfully parsed servers.
func (r DiscoveryResult) TotalServers() int {
	n := 0
	for _, s := range r.Sources {
		n += len(s.Servers)
	}
	return n
}

// --- Discovery source definitions ---

// configFormat selects which top-level key to look for.
type configFormat int

const (
	formatMCPServers configFormat = iota // {"mcpServers": {...}}
	formatServers                        // {"servers": {...}} — VS Code style
)

// supportedSource defines a known MCP config file location.
type supportedSource struct {
	name   string
	path   string       // absolute path resolved at discovery time
	format configFormat // which JSON format to expect
}

// Discover scans the well-known MCP configuration locations and returns
// what it finds. It never executes commands, makes network calls, or
// recursively scans directories.
func Discover() DiscoveryResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return DiscoveryResult{}
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	return DiscoverFromContext(home, cwd)
}

// DiscoverFromContext is the testable variant that accepts explicit home and cwd paths.
func DiscoverFromContext(home, cwd string) DiscoveryResult {
	sources := buildSources(home, cwd)

	result := DiscoveryResult{}
	for _, src := range sources {
		cs := parseSource(src.name, src.path, src.format)
		if cs.Status == SourceStatusNotFound {
			continue // skip silently
		}
		result.Sources = append(result.Sources, cs)
	}
	return result
}

// buildSources constructs the ordered list of config paths to check.
// Paths are platform-specific; no recursive scanning is performed.
func buildSources(home, cwd string) []supportedSource {
	var sources []supportedSource

	// --- Cursor (global) ---
	sources = append(sources, supportedSource{
		name:   "Cursor",
		path:   filepath.Join(home, ".cursor", "mcp.json"),
		format: formatMCPServers,
	})

	// --- Claude Desktop (platform-specific) ---
	claudePath := claudeConfigPath(home)
	if claudePath != "" {
		sources = append(sources, supportedSource{
			name:   "Claude Desktop",
			path:   claudePath,
			format: formatMCPServers,
		})
	}

	// --- VS Code workspace (.vscode/mcp.json) ---
	if cwd != "" {
		sources = append(sources, supportedSource{
			name:   "VS Code (workspace)",
			path:   filepath.Join(cwd, ".vscode", "mcp.json"),
			format: formatServers,
		})

		// --- Portable .mcp.json ---
		sources = append(sources, supportedSource{
			name:   "Portable (.mcp.json)",
			path:   filepath.Join(cwd, ".mcp.json"),
			format: formatMCPServers,
		})
	}

	return sources
}

// claudeConfigPath returns the Claude Desktop config path for the current OS.
func claudeConfigPath(home string) string {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appdata, "Claude", "claude_desktop_config.json")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	default: // linux and others
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		return filepath.Join(xdg, "Claude", "claude_desktop_config.json")
	}
}

// parseSource attempts to read and parse a config file at the given path.
func parseSource(name, path string, format configFormat) ConfigSource {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ConfigSource{Name: name, Path: path, Status: SourceStatusNotFound}
		}
		return ConfigSource{
			Name:   name,
			Path:   path,
			Status: SourceStatusInvalid,
			Detail: fmt.Sprintf("could not open: %v", err),
		}
	}
	defer f.Close()

	var servers []ServerConfig
	var parseErr error
	switch format {
	case formatServers:
		servers, parseErr = ParseVSCodeReader(f)
	default:
		servers, parseErr = ParseConfigReader(f)
	}

	if parseErr != nil {
		return ConfigSource{
			Name:   name,
			Path:   path,
			Status: SourceStatusInvalid,
			Detail: parseErr.Error(),
		}
	}

	return ConfigSource{
		Name:    name,
		Path:    path,
		Status:  SourceStatusFound,
		Servers: servers,
	}
}

// --- JSON parsing: mcpServers format ---

// rawMCPConfig is the JSON structure for mcpServers-style configs.
type rawMCPConfig struct {
	MCPServers json.RawMessage `json:"mcpServers"`
}

// rawVSCodeConfig is the JSON structure for VS Code servers-style configs.
type rawVSCodeConfig struct {
	Servers json.RawMessage `json:"servers"`
}

// rawServerDef is a single server entry (common to both formats).
type rawServerDef struct {
	Command string                     `json:"command"`
	Args    []json.RawMessage          `json:"args"`
	URL     string                     `json:"url"`
	Env     map[string]json.RawMessage `json:"env"`
}

// ParseConfigReader parses an mcpServers-format config from an io.Reader.
// Exported for direct testing.
func ParseConfigReader(r io.Reader) ([]ServerConfig, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var raw rawMCPConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if raw.MCPServers == nil {
		return nil, fmt.Errorf("missing 'mcpServers' field")
	}
	if string(raw.MCPServers) == "null" {
		return nil, fmt.Errorf("'mcpServers' must be an object, got null")
	}

	return parseServersMap(raw.MCPServers)
}

// ParseVSCodeReader parses a VS Code servers-format config from an io.Reader.
// Exported for direct testing.
func ParseVSCodeReader(r io.Reader) ([]ServerConfig, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var raw rawVSCodeConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if raw.Servers == nil {
		return nil, fmt.Errorf("missing 'servers' field")
	}
	if string(raw.Servers) == "null" {
		return nil, fmt.Errorf("'servers' must be an object, got null")
	}

	return parseServersMap(raw.Servers)
}

func parseServersMap(raw json.RawMessage) ([]ServerConfig, error) {
	var serversMap map[string]rawServerDef
	if err := json.Unmarshal(raw, &serversMap); err != nil {
		return nil, fmt.Errorf("servers field must be an object: %w", err)
	}

	var servers []ServerConfig
	for name, def := range serversMap {
		sc, err := classifyServer(name, def)
		if err != nil {
			return nil, fmt.Errorf("server %q: %w", name, err)
		}
		servers = append(servers, sc)
	}

	sortServerConfigs(servers)
	return servers, nil
}

func classifyServer(name string, def rawServerDef) (ServerConfig, error) {
	sc := ServerConfig{
		Name:     name,
		ArgCount: len(def.Args),
		EnvCount: len(def.Env),
	}

	if def.URL != "" {
		sc.Transport = TransportHTTP
		sc.SafeURL = RedactURL(def.URL)
		return sc, nil
	}

	if def.Command != "" {
		sc.Transport = TransportStdio
		sc.Command = def.Command
		return sc, nil
	}

	return ServerConfig{}, fmt.Errorf("server %q has neither 'command' nor 'url'", name)
}

// --- URL secret redaction ---

// sensitiveQueryParams is the set of query parameter names whose values must be redacted.
var sensitiveQueryParams = []string{
	"token", "key", "secret", "password", "pass", "pwd",
	"auth", "authorization", "credential", "api_key", "apikey",
	"access_token", "refresh_token", "client_secret",
}

// RedactURL returns a copy of rawURL with credentials and sensitive query
// parameters replaced by [REDACTED]. It never panics on invalid URLs.
func RedactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		// If we can't parse it at all, suppress entirely rather than leak.
		return "[REDACTED: unparseable URL]"
	}

	// Redact userinfo (e.g. https://user:password@host)
	if u.User != nil {
		_, hasPassword := u.User.Password()
		if hasPassword {
			u.User = url.UserPassword(u.User.Username(), "[REDACTED]")
		}
	}

	// Redact sensitive query parameters
	q := u.Query()
	changed := false
	for _, sensitive := range sensitiveQueryParams {
		// Check both exact and case-insensitive matches
		for key := range q {
			if strings.EqualFold(key, sensitive) {
				q.Set(key, "[REDACTED]")
				changed = true
			}
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}

	return u.String()
}

// sortServerConfigs sorts ServerConfigs by name for deterministic output.
func sortServerConfigs(servers []ServerConfig) {
	for i := 1; i < len(servers); i++ {
		for j := i; j > 0 && servers[j].Name < servers[j-1].Name; j-- {
			servers[j], servers[j-1] = servers[j-1], servers[j]
		}
	}
}

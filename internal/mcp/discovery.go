// Package mcp implements local MCP configuration discovery and static validation.
// It does NOT execute discovered commands or make network connections.
package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	URL       string // for http servers
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

// supportedSource defines a known MCP config file location.
type supportedSource struct {
	name    string
	relPath []string // path components relative to home dir
}

// knownSources lists the well-known MCP client configuration locations.
// These are client-specific files with documented JSON structure.
var knownSources = []supportedSource{
	{name: "Cursor", relPath: []string{".cursor", "mcp.json"}},
	{name: "Claude", relPath: []string{".claude", "claude_desktop_config.json"}},
	{name: "VS Code", relPath: []string{".vscode", "mcp.json"}},
}

// windowsKnownSources are additional Windows-specific locations.
var windowsKnownSources = []supportedSource{
	{name: "Claude (AppData)", relPath: []string{"AppData", "Roaming", "Claude", "claude_desktop_config.json"}},
}

// Discover scans the well-known MCP configuration locations and returns
// what it finds. It never executes commands, makes network calls, or
// recursively scans directories.
func Discover() DiscoveryResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return DiscoveryResult{}
	}
	return DiscoverFromHome(home)
}

// DiscoverFromHome is the testable variant that accepts an explicit home directory.
func DiscoverFromHome(home string) DiscoveryResult {
	sources := knownSources
	if runtime.GOOS == "windows" {
		sources = append(sources, windowsKnownSources...)
	}

	result := DiscoveryResult{}
	for _, src := range sources {
		path := filepath.Join(append([]string{home}, src.relPath...)...)
		cs := parseSource(src.name, path)
		if cs.Status == SourceStatusNotFound {
			continue // skip silently
		}
		result.Sources = append(result.Sources, cs)
	}
	return result
}

// parseSource attempts to read and parse a config file at the given path.
func parseSource(name, path string) ConfigSource {
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

	servers, parseErr := ParseConfigReader(f)
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

// rawMCPConfig is the JSON structure we parse. Only the fields we need.
type rawMCPConfig struct {
	MCPServers json.RawMessage `json:"mcpServers"`
}

// rawServerDef is a single server entry.
type rawServerDef struct {
	Command string                     `json:"command"`
	Args    []json.RawMessage          `json:"args"`
	URL     string                     `json:"url"`
	Env     map[string]json.RawMessage `json:"env"`
}

// ParseConfigReader parses an MCP configuration from an io.Reader.
// It returns a list of ServerConfig items or an error if parsing fails.
// This function is exported for direct testing.
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
	// Reject explicit JSON null (json.RawMessage would be []byte("null"))
	if string(raw.MCPServers) == "null" {
		return nil, fmt.Errorf("'mcpServers' must be an object, got null")
	}

	// mcpServers must be an object, not an array or scalar
	var serversMap map[string]rawServerDef
	if err := json.Unmarshal(raw.MCPServers, &serversMap); err != nil {
		return nil, fmt.Errorf("'mcpServers' must be an object: %w", err)
	}

	var servers []ServerConfig
	for name, def := range serversMap {
		sc, err := classifyServer(name, def)
		if err != nil {
			return nil, fmt.Errorf("server %q: %w", name, err)
		}
		servers = append(servers, sc)
	}

	// Sort for deterministic output
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
		sc.URL = def.URL
		return sc, nil
	}

	if def.Command != "" {
		sc.Transport = TransportStdio
		sc.Command = def.Command
		return sc, nil
	}

	// No command and no URL
	return ServerConfig{}, fmt.Errorf("server %q has neither 'command' nor 'url'", name)
}

// sortServerConfigs sorts ServerConfigs by name for deterministic output.
func sortServerConfigs(servers []ServerConfig) {
	// Simple insertion sort — server lists are small
	for i := 1; i < len(servers); i++ {
		for j := i; j > 0 && servers[j].Name < servers[j-1].Name; j-- {
			servers[j], servers[j-1] = servers[j-1], servers[j]
		}
	}
}

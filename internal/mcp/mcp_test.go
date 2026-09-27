package mcp

import (
	"os"
	"strings"
	"testing"
)

// --- Helpers ---

func configJSON(body string) string {
	return `{"mcpServers":` + body + `}`
}

func parseString(t *testing.T, s string) ([]ServerConfig, error) {
	t.Helper()
	return ParseConfigReader(strings.NewReader(s))
}

// --- 1. No config (empty mcpServers) ---

func TestParse_EmptyServers(t *testing.T) {
	servers, err := parseString(t, configJSON(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

// --- 2. One valid stdio server ---

func TestParse_OneStdioServer(t *testing.T) {
	json := configJSON(`{"filesystem":{"command":"npx","args":["@modelcontextprotocol/server-filesystem","/tmp"]}}`)
	servers, err := parseString(t, json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	s := servers[0]
	if s.Name != "filesystem" {
		t.Errorf("expected name 'filesystem', got %q", s.Name)
	}
	if s.Transport != TransportStdio {
		t.Errorf("expected stdio transport, got %q", s.Transport)
	}
	if s.Command != "npx" {
		t.Errorf("expected command 'npx', got %q", s.Command)
	}
	if s.ArgCount != 2 {
		t.Errorf("expected 2 args, got %d", s.ArgCount)
	}
}

// --- 3. Multiple servers ---

func TestParse_MultipleServers(t *testing.T) {
	json := configJSON(`{
		"server-a": {"command": "a"},
		"server-b": {"command": "b"},
		"server-c": {"command": "c"}
	}`)
	servers, err := parseString(t, json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 3 {
		t.Errorf("expected 3 servers, got %d", len(servers))
	}
	// Verify sorted
	if servers[0].Name > servers[1].Name || servers[1].Name > servers[2].Name {
		t.Errorf("servers not sorted: %v, %v, %v", servers[0].Name, servers[1].Name, servers[2].Name)
	}
}

// --- 4. Malformed JSON ---

func TestParse_MalformedJSON(t *testing.T) {
	_, err := parseString(t, `{"mcpServers": {not valid json}}`)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// --- 5. Missing mcpServers ---

func TestParse_MissingMCPServers(t *testing.T) {
	_, err := parseString(t, `{"other": "data"}`)
	if err == nil {
		t.Error("expected error for missing mcpServers")
	}
}

// --- 6. Invalid mcpServers type (array) ---

func TestParse_InvalidMCPServersType(t *testing.T) {
	_, err := parseString(t, `{"mcpServers": [1, 2, 3]}`)
	if err == nil {
		t.Error("expected error when mcpServers is an array")
	}
}

// --- 7. Missing command (no url either) ---

func TestParse_MissingCommand(t *testing.T) {
	_, err := parseString(t, configJSON(`{"server": {"args": ["foo"]}}`))
	if err == nil {
		t.Error("expected error for server with no command and no url")
	}
}

// --- 8. HTTP-style config ---

func TestParse_HTTPServer(t *testing.T) {
	json := configJSON(`{"myserver": {"url": "http://localhost:3000/mcp"}}`)
	servers, err := parseString(t, json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	s := servers[0]
	if s.Transport != TransportHTTP {
		t.Errorf("expected http transport, got %q", s.Transport)
	}
	if s.URL != "http://localhost:3000/mcp" {
		t.Errorf("unexpected URL: %q", s.URL)
	}
	if s.Command != "" {
		t.Errorf("command should be empty for http server, got %q", s.Command)
	}
}

// --- 9. Unsupported: mcpServers is null ---

func TestParse_NullMCPServers(t *testing.T) {
	_, err := parseString(t, `{"mcpServers": null}`)
	if err == nil {
		t.Error("expected error for null mcpServers")
	}
}

// --- 10. Multiple config sources (DiscoverFromHome) ---

func TestDiscoverFromHome_Multiple(t *testing.T) {
	home := t.TempDir()

	// Create two different config files
	writeConfig(t, home, ".cursor", "mcp.json", configJSON(`{"s1":{"command":"npx"}}`))
	writeConfig(t, home, ".vscode", "mcp.json", configJSON(`{"s2":{"command":"node"}}`))

	result := DiscoverFromHome(home)

	found := 0
	for _, src := range result.Sources {
		if src.Status == SourceStatusFound {
			found++
		}
	}
	if found < 2 {
		t.Errorf("expected at least 2 found sources, got %d", found)
	}
	if result.TotalServers() < 2 {
		t.Errorf("expected at least 2 total servers, got %d", result.TotalServers())
	}
}

// --- 11. Secret safety: env values must never appear in ServerConfig ---

func TestParse_EnvSecretsNotExposed(t *testing.T) {
	json := configJSON(`{
		"test": {
			"command": "server",
			"env": {
				"API_KEY": "super-secret-value",
				"TOKEN": "another-secret",
				"NORMAL_VAR": "some-value"
			}
		}
	}`)

	servers, err := parseString(t, json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	s := servers[0]
	// EnvCount should reflect the number, but values must not be stored or printable
	if s.EnvCount != 3 {
		t.Errorf("expected EnvCount=3, got %d", s.EnvCount)
	}

	// Verify secret values are not in the struct string representation
	serverStr := s.Name + s.Command + s.URL + string(s.Transport)
	if strings.Contains(serverStr, "super-secret-value") {
		t.Error("secret value leaked into ServerConfig")
	}
	if strings.Contains(serverStr, "another-secret") {
		t.Error("secret token leaked into ServerConfig")
	}
}

// --- 12. No config files = no sources in result (not error) ---

func TestDiscoverFromHome_NoFiles(t *testing.T) {
	home := t.TempDir() // empty temp dir
	result := DiscoverFromHome(home)

	if result.HasAny() {
		t.Error("expected no discovered sources in empty home")
	}
	if result.HasFailure() {
		t.Error("expected no failures when no files exist")
	}
}

// --- 13. Malformed config file in discovered path ---

func TestDiscoverFromHome_MalformedFile(t *testing.T) {
	home := t.TempDir()
	writeConfig(t, home, ".cursor", "mcp.json", `not json at all`)

	result := DiscoverFromHome(home)
	if !result.HasAny() {
		t.Error("expected source to be found (even if invalid)")
	}
	if !result.HasFailure() {
		t.Error("expected failure for malformed JSON")
	}
}

// --- 14. Env count is safe even with many vars ---

func TestParse_EnvCount(t *testing.T) {
	json := configJSON(`{"srv": {"command": "bin", "env": {"A":"1","B":"2","C":"3","D":"4","E":"5"}}}`)
	servers, err := parseString(t, json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if servers[0].EnvCount != 5 {
		t.Errorf("expected EnvCount=5, got %d", servers[0].EnvCount)
	}
}

// --- Helper ---

func writeConfig(t *testing.T, home string, subdir, file, content string) {
	t.Helper()
	dir := home + "/" + subdir
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/"+file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

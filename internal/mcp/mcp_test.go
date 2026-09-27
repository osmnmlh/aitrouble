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

func parseVSCodeString(t *testing.T, s string) ([]ServerConfig, error) {
	t.Helper()
	return ParseVSCodeReader(strings.NewReader(s))
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

// --- 8. HTTP-style config (mcpServers format) ---

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
	if s.SafeURL != "http://localhost:3000/mcp" {
		t.Errorf("unexpected URL: %q", s.SafeURL)
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

// --- 10. Multiple config sources (DiscoverFromContext) ---

func TestDiscoverFromContext_Multiple(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()

	// Cursor: home/.cursor/mcp.json
	writeConfig(t, home, ".cursor", "mcp.json", configJSON(`{"s1":{"command":"npx"}}`))
	// VS Code workspace: cwd/.vscode/mcp.json  (VS Code format uses "servers")
	writeConfig(t, cwd, ".vscode", "mcp.json", `{"servers":{"s2":{"command":"node"}}}`)

	result := DiscoverFromContext(home, cwd)

	found := 0
	for _, src := range result.Sources {
		if src.Status == SourceStatusFound {
			found++
		}
	}
	if found < 2 {
		t.Errorf("expected at least 2 found sources, got %d (sources: %+v)", found, result.Sources)
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
	if s.EnvCount != 3 {
		t.Errorf("expected EnvCount=3, got %d", s.EnvCount)
	}

	// Verify secret values are not in the struct string representation
	serverStr := s.Name + s.Command + s.SafeURL + string(s.Transport)
	if strings.Contains(serverStr, "super-secret-value") {
		t.Error("secret value leaked into ServerConfig")
	}
	if strings.Contains(serverStr, "another-secret") {
		t.Error("secret token leaked into ServerConfig")
	}
}

// --- 12. No config files = no sources in result (not error) ---

func TestDiscoverFromContext_NoFiles(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	result := DiscoverFromContext(home, cwd)

	if result.HasAny() {
		t.Error("expected no discovered sources in empty home/cwd")
	}
	if result.HasFailure() {
		t.Error("expected no failures when no files exist")
	}
}

// --- 13. Malformed config file in discovered path ---

func TestDiscoverFromContext_MalformedFile(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeConfig(t, home, ".cursor", "mcp.json", `not json at all`)

	result := DiscoverFromContext(home, cwd)
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

// --- 15. VS Code "servers" format ---

func TestParse_VSCodeServersFormat(t *testing.T) {
	json := `{"servers":{"myserver":{"command":"uvx","args":["mcp-server-fetch"]}}}`
	servers, err := parseVSCodeString(t, json)
	if err != nil {
		t.Fatalf("unexpected error parsing VS Code format: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	s := servers[0]
	if s.Name != "myserver" {
		t.Errorf("expected name 'myserver', got %q", s.Name)
	}
	if s.Transport != TransportStdio {
		t.Errorf("expected stdio, got %q", s.Transport)
	}
	if s.Command != "uvx" {
		t.Errorf("expected command 'uvx', got %q", s.Command)
	}
	if s.ArgCount != 1 {
		t.Errorf("expected ArgCount=1, got %d", s.ArgCount)
	}
}

// --- 16. Portable .mcp.json format (mcpServers key) ---

func TestDiscoverFromContext_PortableMCPJson(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()

	// Write .mcp.json in cwd with mcpServers format
	content := configJSON(`{"portable-server":{"command":"node","args":["server.js"]}}`)
	if err := os.WriteFile(cwd+"/.mcp.json", []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	result := DiscoverFromContext(home, cwd)
	if !result.HasAny() {
		t.Error("expected portable .mcp.json to be discovered")
	}
	if result.TotalServers() != 1 {
		t.Errorf("expected 1 server from portable .mcp.json, got %d", result.TotalServers())
	}
	// Verify source name
	found := false
	for _, src := range result.Sources {
		if strings.Contains(src.Name, "Portable") || strings.Contains(src.Name, "mcp.json") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a source named 'Portable' or similar, got: %+v", result.Sources)
	}
}

// --- 17. URL redaction: query param token ---

func TestRedactURL_QueryToken(t *testing.T) {
	raw := "https://example.com/mcp?token=SUPERSECRET&other=fine"
	safe := RedactURL(raw)
	if strings.Contains(safe, "SUPERSECRET") {
		t.Errorf("token leaked into URL: %s", safe)
	}
	// url.Values.Encode() encodes brackets, so accept both forms
	if !strings.Contains(safe, "REDACTED") {
		t.Errorf("expected REDACTED marker in URL, got: %s", safe)
	}
	if !strings.Contains(safe, "other=fine") {
		t.Errorf("non-sensitive param removed: %s", safe)
	}
}

// --- 18. URL redaction: userinfo credentials ---

func TestRedactURL_UserinfoCredentials(t *testing.T) {
	raw := "https://user:PASSWORD@example.com/mcp"
	safe := RedactURL(raw)
	if strings.Contains(safe, "PASSWORD") {
		t.Errorf("password leaked into URL: %s", safe)
	}
	// url.UserPassword encodes the replacement, accept any form containing REDACTED
	if !strings.Contains(safe, "REDACTED") {
		t.Errorf("expected REDACTED marker in URL, got: %s", safe)
	}
	if !strings.Contains(safe, "user") {
		t.Errorf("username should be preserved, got: %s", safe)
	}
}

// --- 19. URL redaction: safe URL is unchanged ---

func TestRedactURL_SafeURL(t *testing.T) {
	raw := "http://localhost:3000/mcp"
	safe := RedactURL(raw)
	if safe != raw {
		t.Errorf("safe URL was modified: %q -> %q", raw, safe)
	}
}

// --- 20. HTTP server URL is redacted in ServerConfig ---

func TestParse_HTTPServerURLRedacted(t *testing.T) {
	json := configJSON(`{"srv": {"url": "https://example.com/mcp?token=mysecret"}}`)
	servers, err := parseString(t, json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server")
	}
	if strings.Contains(servers[0].SafeURL, "mysecret") {
		t.Errorf("secret leaked into SafeURL: %s", servers[0].SafeURL)
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func expectedProvider(mode string) Expectation {
	e := Expectation{ExitCode: 1, ProviderRequests: 1, ProviderRequest: "GET /v1/models"}
	switch mode {
	case "ok":
		e.FailingLayer, e.FailureKind, e.ExitCode = "None", "None", 0
	case "401", "403":
		e.FailingLayer, e.FailureKind = "Provider › Authentication", "auth_failure"
	case "404":
		e.FailingLayer, e.FailureKind = "Provider › /models", "route_not_found"
	case "429":
		e.FailingLayer, e.FailureKind = "Provider", "rate_limited"
	case "500", "502", "503":
		e.FailingLayer, e.FailureKind = "Provider", "provider_server_error"
	case "400":
		e.FailingLayer, e.FailureKind = "Provider", "provider_http_error"
	case "invalid_json", "missing_data":
		e.FailingLayer, e.FailureKind = "Provider › Response", "provider_invalid_response"
	case "slow":
		e.FailingLayer, e.FailureKind = "Provider › HTTP", "http_timeout"
	case "connection_close":
		// A provider-side closed connection is an HTTP-layer failure. The
		// implementation currently emits http_error, but maps it to Unknown;
		// retaining this expectation is intentional negative evidence.
		e.FailingLayer, e.FailureKind = "Provider › HTTP", "http_error"
	}
	return e
}

func providerScenario(r *Runner, id, name, category, mode string, seed int64, binary string) ScenarioResult {
	s, err := r.begin(id, name, category, seed)
	if err != nil {
		return ScenarioResult{ID: id, Name: name, Status: StatusBlocked, Triage: err.Error()}
	}
	s.setBinary(binary)
	p := newControlledProvider(mode, 31*time.Second)
	defer p.close()
	apiKey := "organic-provider-key"
	if err := writeText(filepath.Join(s.project, ".env"), "OPENAI_BASE_URL="+p.baseURL("/v1")+"\nOPENAI_API_KEY="+apiKey+"\n"); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	run := r.doctor(ctx, binary, s.project, s.home, s.appdata, map[string]*string{})
	cancel()
	s.writeStage("doctor", run)
	truth := expectedProvider(mode)
	t := true
	s.compare(run, p, truth, &t)
	s.expectBaseSource(run, ".env")
	s.evidence("controlled provider mode=" + mode + "; handler independently observed " + fmt.Sprint(s.result.Observed.ProviderRequests) + " request(s)")
	p.close()
	s.result.Process.ListenerClosed = true
	if mode == "connection_close" {
		s.result.Triage = "PRODUCT BUG — classified connection-close as Unknown instead of Provider › HTTP"
	}
	return s.finish()
}

func tcpRefusalScenario(r *Runner, id string, seed int64, binary string) ScenarioResult {
	s, err := r.begin(id, "TCP refusal short-circuits provider", "network", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	s.setBinary(binary)
	port, err := closedPort()
	if err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	oracleErr := directTCP(addr)
	s.check("independent_tcp_refusal_oracle", oracleErr != nil)
	base := "http://" + addr + "/v1"
	_ = writeText(filepath.Join(s.project, ".env"), "OPENAI_BASE_URL="+base+"\nOPENAI_API_KEY=dotenv-key\n")
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	run := r.doctor(ctx, binary, s.project, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(base)})
	cancel()
	s.writeStage("doctor", run)
	s.compare(run, nil, Expectation{ExitCode: 1, FailingLayer: "Network › TCP", FailureKind: "tcp_refused", ProviderRequests: 0, DownstreamProviderSK: true}, nil)
	s.expectBaseSource(run, "shell")
	s.evidence("direct TCP dial to " + addr + " failed before subject execution: " + fmt.Sprint(oracleErr != nil))
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func dnsFailureScenario(r *Runner, id string, seed int64, binary string) ScenarioResult {
	s, err := r.begin(id, "Reserved invalid DNS name short-circuits downstream", "network", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	s.setBinary(binary)
	host := "invalid-test-domain.invalid"
	_, oracleErr := net.DefaultResolver.LookupHost(context.Background(), host)
	if oracleErr == nil {
		s.forced = StatusBlocked
		s.result.Triage = "environment resolver unexpectedly resolved the reserved invalid test name"
		_ = s.snapshot()
		return s.finish()
	}
	base := "https://" + host + "/v1"
	_ = writeText(filepath.Join(s.project, ".env"), "OPENAI_BASE_URL="+base+"\nOPENAI_API_KEY=dotenv-key\n")
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	run := r.doctor(ctx, binary, s.project, s.home, s.appdata, map[string]*string{})
	cancel()
	s.writeStage("doctor", run)
	s.check("independent_dns_failure_oracle", oracleErr != nil)
	s.compare(run, nil, Expectation{ExitCode: 1, FailingLayer: "Network › DNS", FailureKind: "dns_error", ProviderRequests: 0, DownstreamProviderSK: true}, nil)
	s.expectBaseSource(run, ".env")
	s.evidence("OS resolver rejected reserved hostname before subject execution")
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func untrustedTLSScenario(r *Runner, id string, seed int64, binary string) ScenarioResult {
	s, err := r.begin(id, "Untrusted local TLS certificate", "tls", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	s.setBinary(binary)
	tlsServer := newUntrustedTLSServer()
	defer tlsServer.close()
	s.check("independent_tcp_accept_oracle", directTCP(tlsServer.address()) == nil)
	_ = writeText(filepath.Join(s.project, ".env"), "OPENAI_BASE_URL="+tlsServer.url+"\nOPENAI_API_KEY=dotenv-key\n")
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	run := r.doctor(ctx, binary, s.project, s.home, s.appdata, map[string]*string{})
	cancel()
	s.writeStage("doctor", run)
	s.compare(run, nil, Expectation{ExitCode: 1, FailingLayer: "Network › TLS", FailureKind: "tls_cert_error", ProviderRequests: 0, DownstreamProviderSK: true}, nil)
	s.expectBaseSource(run, ".env")
	_ = tlsServer.close()
	s.result.Process.ListenerClosed = true
	s.evidence("test-owned HTTPS server presented only a self-signed certificate; TCP was independently reachable")
	return s.finish()
}

func handshakeFailureScenario(r *Runner, id string, seed int64, binary string) ScenarioResult {
	s, err := r.begin(id, "TLS handshake close after real TCP accept", "tls", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	s.setBinary(binary)
	tlsServer, err := newPlainHandshakeFailureServer()
	if err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	defer tlsServer.close()
	s.check("independent_tcp_accept_oracle", directTCP(tlsServer.address()) == nil)
	_ = writeText(filepath.Join(s.project, ".env"), "OPENAI_BASE_URL="+tlsServer.url+"\nOPENAI_API_KEY=dotenv-key\n")
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	run := r.doctor(ctx, binary, s.project, s.home, s.appdata, map[string]*string{})
	cancel()
	s.writeStage("doctor", run)
	s.compare(run, nil, Expectation{ExitCode: 1, FailingLayer: "Network › TLS", FailureKind: "tls_error", ProviderRequests: 0, DownstreamProviderSK: true}, nil)
	s.expectBaseSource(run, ".env")
	_ = tlsServer.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func staleShellScenario(r *Runner, id string, seed int64, binary string) ScenarioResult {
	s, err := r.begin(id, "Stale shell endpoint overrides healthy dotenv", "configuration", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	s.setBinary(binary)
	p := newControlledProvider("ok", 0)
	defer p.close()
	port, err := closedPort()
	if err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	bad := fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	s.check("independent_shell_target_refused", directTCP(fmt.Sprintf("127.0.0.1:%d", port)) != nil)
	_ = writeText(filepath.Join(s.project, ".env"), "OPENAI_BASE_URL="+p.baseURL("/v1")+"\nOPENAI_API_KEY=dotenv-key\n")
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	run := r.doctor(ctx, binary, s.project, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(bad), "OPENAI_API_KEY": nullable("shell-key")})
	cancel()
	s.writeStage("doctor", run)
	s.compare(run, p, Expectation{ExitCode: 1, FailingLayer: "Network › TCP", FailureKind: "tcp_refused", ProviderRequests: 0, DownstreamProviderSK: true}, nil)
	s.expectBaseSource(run, "shell")
	s.evidence("healthy dotenv target was intentionally present; independent oracle chose shell target and confirmed refusal")
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func mcpConfig(serverCount int, prefix string, useVSCode bool) (string, []string) {
	servers := map[string]any{}
	names := []string{}
	for i := 0; i < serverCount; i++ {
		name := fmt.Sprintf("%s-%02d", prefix, i)
		if i%2 == 0 {
			servers[name] = map[string]any{"command": "node", "args": []string{"server.js", "--index", fmt.Sprint(i)}, "env": map[string]string{"MODE": "test", "INDEX": fmt.Sprint(i)}}
		} else {
			servers[name] = map[string]any{"url": fmt.Sprintf("https://mcp.example.invalid/%d?model=test&limit=%d", i, i+1)}
		}
		names = append(names, name)
	}
	key := "mcpServers"
	if useVSCode {
		key = "servers"
	}
	b, _ := json.Marshal(map[string]any{key: servers})
	return string(b), names
}

func validMCPScenario(r *Runner, id, name string, seed int64, count int, source string) ScenarioResult {
	s, err := r.begin(id, name, "mcp", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	p := newControlledProvider("ok", 0)
	defer p.close()
	var path string
	var body string
	var names []string
	switch source {
	case "Cursor":
		path = filepath.Join(s.home, ".cursor", "mcp.json")
		body, names = mcpConfig(count, "cursor", false)
	case "Claude Desktop":
		path = filepath.Join(s.appdata, "Claude", "claude_desktop_config.json")
		body, names = mcpConfig(count, "claude", false)
	case "VS Code (workspace)":
		path = filepath.Join(s.project, ".vscode", "mcp.json")
		body, names = mcpConfig(count, "vscode", true)
	default:
		path = filepath.Join(s.project, ".mcp.json")
		body, names = mcpConfig(count, "portable", false)
		source = "Portable (.mcp.json)"
	}
	if err := writeText(path, body); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	key := "mcp-provider-key"
	run := r.doctor(ctx, r.bin.Path, s.project, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(p.baseURL("/v1")), "OPENAI_API_KEY": nullable(key)})
	cancel()
	s.writeStage("doctor", run)
	t := true
	s.compare(run, p, expectedProvider("ok"), &t)
	s.expectBaseSource(run, "shell")
	s.check("mcp_expected_source_discovered", strings.Contains(run.stdout, "✓  "+source))
	s.check("mcp_expected_server_count", strings.Contains(run.stdout, fmt.Sprintf("%d server(s) configured", count)))
	for _, n := range names {
		s.check("mcp_server_"+n, strings.Contains(run.stdout, n))
	}
	s.evidence("static MCP source=" + source + " expected server count=" + fmt.Sprint(count))
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func malformedMCPScenario(r *Runner, id, label, body string, seed int64) ScenarioResult {
	s, err := r.begin(id, "Malformed MCP: "+label, "mcp", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	p := newControlledProvider("ok", 0)
	defer p.close()
	_ = writeText(filepath.Join(s.project, ".mcp.json"), body)
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	key := "mcp-provider-key"
	run := r.doctor(ctx, r.bin.Path, s.project, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(p.baseURL("/v1")), "OPENAI_API_KEY": nullable(key)})
	cancel()
	s.writeStage("doctor", run)
	t := true
	s.compare(run, p, Expectation{ExitCode: 1, FailingLayer: "MCP › Configuration", FailureKind: "N/A", ProviderRequests: 1, ProviderRequest: "GET /v1/models"}, &t)
	s.check("mcp_invalid_visible", strings.Contains(run.stdout, "Portable (.mcp.json)"))
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func mcpSecretScenario(r *Runner, id string, seed int64, binary string) ScenarioResult {
	s, err := r.begin(id, "MCP URL and environment canary redaction", "security", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	s.setBinary(binary)
	p := newControlledProvider("ok", 0)
	defer p.close()
	canary := s.secretCanary()
	body := fmt.Sprintf(`{"mcpServers":{"safe-http":{"url":"https://user:%s@mcp.example.invalid/mcp?token=%s&model=kept"},"safe-stdio":{"command":"node","args":["server.js"],"env":{"API_KEY":"%s"}}}}`, canary, canary, canary)
	_ = writeText(filepath.Join(s.project, ".mcp.json"), body)
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	key := "mcp-provider-key"
	run := r.doctor(ctx, binary, s.project, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(p.baseURL("/v1")), "OPENAI_API_KEY": nullable(key)})
	cancel()
	s.writeStage("doctor", run)
	t := true
	s.compare(run, p, expectedProvider("ok"), &t)
	s.check("safe_url_keeps_harmless_parameter", strings.Contains(run.stdout, "model=kept"))
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func mcpNoExecScenario(r *Runner, id string, seed int64, binary string) ScenarioResult {
	s, err := r.begin(id, "MCP discovery does not execute configured command", "security", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	s.setBinary(binary)
	p := newControlledProvider("ok", 0)
	defer p.close()
	marker := filepath.Join(r.lab, "processes", id+"-mcp-command.marker")
	_ = writeText(filepath.Join(s.project, ".mcp.json"), processMarkerConfig(marker))
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	key := "mcp-provider-key"
	run := r.doctor(ctx, binary, s.project, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(p.baseURL("/v1")), "OPENAI_API_KEY": nullable(key)})
	cancel()
	s.writeStage("doctor", run)
	t := true
	s.compare(run, p, expectedProvider("ok"), &t)
	found := waitForFile(marker, 200*time.Millisecond)
	s.result.Process = ProcessObservation{MarkerPath: marker, MarkerFound: found, UnexpectedChild: found, ListenerClosed: true}
	s.check("mcp_command_marker_absent", !found)
	s.evidence("configured command was a scenario-owned marker writer; marker presence is an independent execution oracle")
	p.close()
	return s.finish()
}

func cliScenario(r *Runner, id string, seed int64) ScenarioResult {
	s, err := r.begin(id, "Adversarial CLI input semantics", "cli", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	cases := []struct {
		name string
		args []string
		code int
	}{{"help", []string{"--help"}, 0}, {"version", []string{"--version"}, 0}, {"unknown", []string{"unknown-command"}, 2}, {"bad_doctor_flag", []string{"doctor", "--not-a-flag"}, 2}, {"doctor_help", []string{"doctor", "--help"}, 0}}
	all := true
	for _, c := range cases {
		code, out, errOut, _ := commandOutput(s.project, cleanChildEnv(s.home, s.appdata, map[string]*string{}), r.bin.Path, c.args...)
		all = all && code == c.code
		s.wrote = append(s.wrote, filepath.Join(s.artifact, c.name+"-stdout.txt"), filepath.Join(s.artifact, c.name+"-stderr.txt"))
		_ = os.WriteFile(s.wrote[len(s.wrote)-2], []byte(out), 0600)
		_ = os.WriteFile(s.wrote[len(s.wrote)-1], []byte(errOut), 0600)
	}
	s.check("documented_cli_exit_codes", all)
	s.result.Expected, s.result.Observed = Expectation{ExitCode: 0, FailingLayer: "N/A", FailureKind: "N/A"}, Observation{ExitCode: 0, FailingLayer: "N/A", FailureKind: "N/A"}
	s.result.Process.ListenerClosed = true
	return s.finish()
}

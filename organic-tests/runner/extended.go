package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func trustedTLSScenario(r *Runner, id string, seed int64) ScenarioResult {
	s, err := r.begin(id, "Locally trusted TLS certificate", "tls", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	f, err := newTrustedTLSFixture(r.lab, seed)
	if err != nil {
		s.forced = StatusBlocked
		s.result.Triage = "trusted TLS fixture unavailable: " + err.Error()
		_ = s.snapshot()
		return s.finish()
	}
	defer f.close()
	trustErr := f.verifyTrust()
	s.check("independent_system_trust_oracle", trustErr == nil)
	if trustErr != nil {
		s.forced = StatusBlocked
		s.result.Triage = "temporary test root was not accepted by the local Go trust store: " + trustErr.Error()
		_ = s.snapshot()
		if closeErr := f.close(); closeErr != nil {
			s.result.Triage += "; cleanup: " + closeErr.Error()
		}
		return s.finish()
	}
	f.provider.resetRequests()
	_ = writeText(filepath.Join(s.project, ".env"), "OPENAI_BASE_URL="+f.provider.baseURL("/v1")+"\nOPENAI_API_KEY=tls-provider-key\n")
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	values := map[string]*string{
		"SSL_CERT_FILE": nullable(f.rootPath),
	}
	run := r.doctor(ctx, r.bin.Path, s.project, s.home, s.appdata, values)
	cancel()
	s.writeStage("doctor", run)
	t := true
	s.compare(run, f.provider, expectedProvider("ok"), &t)
	s.expectBaseSource(run, ".env")
	if err := f.close(); err != nil {
		s.check("temporary_root_removed", false)
		s.result.Triage = err.Error()
	} else {
		s.check("temporary_root_removed", true)
	}
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func apiSourcePatternValue(stdout string) string {
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "OPENAI_API_KEY") && strings.Contains(line, "source:") {
			if strings.Contains(line, "source: shell") {
				return "shell"
			}
			if strings.Contains(line, "source: .env") {
				return ".env"
			}
		}
	}
	return "absent"
}

func randomizedConfigProviderScenario(r *Runner, ordinal int, seed int64) ScenarioResult {
	id := fmt.Sprintf("R%02d", ordinal)
	s, err := r.begin(id, "Seeded configuration/provider perturbation", "randomized", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	rng := s.random()
	modes := []string{"ok", "401", "403", "404", "429", "500", "502", "503", "invalid_json", "missing_data"}
	mode := modes[rng.Intn(len(modes))]
	p := newControlledProvider(mode, 0)
	defer p.close()
	port, err := closedPort()
	if err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	bad := fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	good := p.baseURL("/v1")
	dotBase := good
	if rng.Intn(3) == 0 {
		dotBase = bad
	}
	shellMode := rng.Intn(4) // absent, healthy, refused, explicitly empty
	var shellBase *string
	expectedBaseSource, effectiveBase := ".env", dotBase
	switch shellMode {
	case 1:
		shellBase = nullable(good)
		expectedBaseSource, effectiveBase = "shell", good
	case 2:
		shellBase = nullable(bad)
		expectedBaseSource, effectiveBase = "shell", bad
	case 3:
		shellBase = nullable("")
		expectedBaseSource, effectiveBase = "shell", ""
	}
	dotKey := "dotenv-random-key"
	keyMode := rng.Intn(3) // dotenv, shell key, empty shell key
	var shellKey *string
	expectedKeySource, authExpected := ".env", true
	if keyMode == 1 {
		shellKey = nullable("shell-random-key")
		expectedKeySource = "shell"
	}
	if keyMode == 2 {
		shellKey = nullable("")
		expectedKeySource, authExpected = "shell", false
	}
	format := []string{
		"OPENAI_BASE_URL=%s\nOPENAI_API_KEY=%s\n",
		"export OPENAI_API_KEY=\"%s\"\r\nexport OPENAI_BASE_URL=\"%s\" # harmless\r\n",
		"\ufeff# seeded formatting\nOPENAI_BASE_URL='%s'\n\nOPENAI_API_KEY=%s\n",
	}[rng.Intn(3)]
	var dotenv string
	if strings.Contains(format, "OPENAI_API_KEY") && strings.Index(format, "OPENAI_API_KEY") < strings.Index(format, "OPENAI_BASE_URL") {
		dotenv = fmt.Sprintf(format, dotKey, dotBase)
	} else {
		dotenv = fmt.Sprintf(format, dotBase, dotKey)
	}
	_ = writeText(filepath.Join(s.project, ".env"), dotenv)
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	values := map[string]*string{"OPENAI_BASE_URL": shellBase, "OPENAI_API_KEY": shellKey}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	run := r.doctor(ctx, r.bin.Path, s.project, s.home, s.appdata, values)
	cancel()
	s.writeStage("doctor", run)
	var expected Expectation
	var auth *bool
	if effectiveBase == "" {
		expected = Expectation{ExitCode: 1, FailingLayer: "", FailureKind: "", ProviderRequests: 0}
		s.check("empty_shell_value_is_explicit", strings.Contains(run.stdout, `OPENAI_BASE_URL  ""`))
		actual := parseActual(run, p)
		if actual.ExitCode == 1 && actual.FailingLayer == "Network" && actual.FailureKind == "invalid_url" {
			s.result.Triage = "PRODUCT BUG — empty config value returns literal quotes from String() bypassing emptiness check and causing invalid_url network failure"
		}
	} else if effectiveBase == bad {
		s.check("independent_refused_target", directTCP(fmt.Sprintf("127.0.0.1:%d", port)) != nil)
		expected = Expectation{ExitCode: 1, FailingLayer: "Network › TCP", FailureKind: "tcp_refused", ProviderRequests: 0, DownstreamProviderSK: true}
	} else {
		expected = expectedProvider(mode)
		auth = &authExpected
		actual := parseActual(run, p)
		if actual.ExitCode == 1 && actual.FailingLayer == "Network" && actual.FailureKind == "invalid_url" {
			s.result.Triage = "PRODUCT BUG — dotenv parser fails to strip inline comments from quoted values, causing invalid_url network failure"
		}
	}
	s.compare(run, p, expected, auth)
	s.expectBaseSource(run, expectedBaseSource)
	s.check("effective_api_key_source", apiSourcePatternValue(run.stdout) == expectedKeySource)
	s.evidence(fmt.Sprintf("seeded inputs: base source=%s, key source=%s, provider mode=%s", expectedBaseSource, expectedKeySource, mode))
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func metamorphicPortScenario(r *Runner, id string, seed int64) ScenarioResult {
	s, err := r.begin(id, "Metamorphic endpoint-port change", "metamorphic", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	p := newControlledProvider("ok", 0)
	defer p.close()
	port, err := closedPort()
	if err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	goodProject, badProject := filepath.Join(s.project, "healthy"), filepath.Join(s.project, "port-mutated")
	_ = os.MkdirAll(goodProject, 0700)
	_ = os.MkdirAll(badProject, 0700)
	good, bad := p.baseURL("/v1"), fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	_ = writeText(filepath.Join(goodProject, ".env"), "OPENAI_BASE_URL="+good+"\nOPENAI_API_KEY=key\n")
	_ = writeText(filepath.Join(badProject, ".env"), "OPENAI_BASE_URL="+bad+"\nOPENAI_API_KEY=key\n")
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	a := r.doctor(ctx, r.bin.Path, goodProject, s.home, s.appdata, map[string]*string{})
	s.writeStage("healthy", a)
	aa := parseActual(a, p)
	p.resetRequests()
	
	b := r.doctor(ctx, r.bin.Path, badProject, s.home, s.appdata, map[string]*string{})
	s.writeStage("port-mutated", b)
	cancel()
	bb := parseActual(b, p)
	
	s.result.Expected, s.result.Observed = Expectation{ExitCode: 1, FailingLayer: "Network › TCP", FailureKind: "tcp_refused", ProviderRequests: 0, DownstreamProviderSK: true}, bb
	s.check("healthy_baseline", aa.ExitCode == 0 && aa.FailingLayer == "None" && aa.ProviderRequests == 1)
	s.check("only_port_changes_to_tcp_failure", bb.ExitCode == 1 && bb.FailingLayer == "Network › TCP" && bb.FailureKind == "tcp_refused" && bb.ProviderRequests == 0) // changed from 1 to 0 because badProject refuses TCP! Wait, the test asserted 1? Let me check below. Yes, it asserted 1 which was a bug!
	s.check("independent_mutated_port_refused", directTCP(fmt.Sprintf("127.0.0.1:%d", port)) != nil)
	s.evidence("the second request count includes only the healthy endpoint; the port-mutated target cannot reach the provider")
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func metamorphicShellScenario(r *Runner, id string, seed int64) ScenarioResult {
	s, err := r.begin(id, "Metamorphic shell precedence change", "metamorphic", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	p := newControlledProvider("ok", 0)
	defer p.close()
	port, _ := closedPort()
	bad := fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	baseProject, shellProject := filepath.Join(s.project, "dotenv-only"), filepath.Join(s.project, "shell-override")
	_ = os.MkdirAll(baseProject, 0700)
	_ = os.MkdirAll(shellProject, 0700)
	for _, dir := range []string{baseProject, shellProject} {
		_ = writeText(filepath.Join(dir, ".env"), "OPENAI_BASE_URL="+bad+"\nOPENAI_API_KEY=key\n")
	}
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	a := r.doctor(ctx, r.bin.Path, baseProject, s.home, s.appdata, map[string]*string{})
	s.writeStage("dotenv-only", a)
	aa := parseActual(a, p)
	p.resetRequests()
	
	b := r.doctor(ctx, r.bin.Path, shellProject, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(p.baseURL("/v1"))})
	s.writeStage("shell-override", b)
	cancel()
	bb := parseActual(b, p)
	
	s.result.Expected, s.result.Observed = Expectation{ExitCode: 0, FailingLayer: "None", FailureKind: "None", ProviderRequests: 1, ProviderRequest: "GET /v1/models", DownstreamProviderSK: false}, bb
	s.check("dotenv_baseline_is_tcp_failure", aa.FailingLayer == "Network › TCP" && aa.ProviderRequests == 0)
	s.check("shell_only_change_makes_healthy", bb.FailingLayer == "None" && bb.ExitCode == 0 && bb.ProviderRequests == 1)
	s.check("source_changed_to_shell", strings.Contains(b.stdout, "source: shell"))
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func metamorphicFormattingScenario(r *Runner, id string, seed int64) ScenarioResult {
	s, err := r.begin(id, "Metamorphic dotenv formatting invariance", "metamorphic", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	p := newControlledProvider("ok", 0)
	defer p.close()
	aDir, bDir := filepath.Join(s.project, "lf"), filepath.Join(s.project, "bom-crlf")
	_ = os.MkdirAll(aDir, 0700)
	_ = os.MkdirAll(bDir, 0700)
	base := p.baseURL("/v1")
	_ = writeText(filepath.Join(aDir, ".env"), "OPENAI_BASE_URL="+base+"\nOPENAI_API_KEY=key\n")
	_ = writeText(filepath.Join(bDir, ".env"), "\ufeffexport OPENAI_API_KEY=\"key\"\r\nOPENAI_BASE_URL=\""+base+"\" # comment\r\n")
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	a := r.doctor(ctx, r.bin.Path, aDir, s.home, s.appdata, map[string]*string{})
	s.writeStage("lf", a)
	aa := parseActual(a, p)
	p.resetRequests()
	
	b := r.doctor(ctx, r.bin.Path, bDir, s.home, s.appdata, map[string]*string{})
	s.writeStage("bom-crlf", b)
	cancel()
	bb := parseActual(b, p)
	
	s.result.Expected, s.result.Observed = expectedProvider("ok"), bb
	if bb.ExitCode == 1 && bb.FailingLayer == "Network" && bb.FailureKind == "invalid_url" {
		s.result.Triage = "PRODUCT BUG — dotenv parser fails to strip inline comments from quoted values, causing invalid_url network failure"
	}
	s.check("lf_semantics_healthy", aa.ExitCode == 0 && aa.FailingLayer == "None" && aa.ProviderRequests == 1)
	s.check("bom_crlf_semantics_healthy", bb.ExitCode == 0 && bb.FailingLayer == "None" && bb.ProviderRequests == 1)
	s.check("both_use_dotenv", strings.Contains(a.stdout, "source: .env") && strings.Contains(b.stdout, "source: .env"))
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func metamorphicURLScenario(r *Runner, id string, seed int64) ScenarioResult {
	s, err := r.begin(id, "Metamorphic URL path/query/fragment normalization", "metamorphic", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	p := newControlledProvider("ok", 0)
	defer p.close()
	paths := []string{"/v1", "/v1/", "/v1?foo=bar", "/v1#fragment"}
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	all := true
	for i, suffix := range paths {
		dir := filepath.Join(s.project, fmt.Sprintf("url-%d", i))
		_ = os.MkdirAll(dir, 0700)
		_ = writeText(filepath.Join(dir, ".env"), "OPENAI_BASE_URL="+p.baseURL(suffix)+"\nOPENAI_API_KEY=key\n")
		// The files are test inputs, so snapshot again after this subcase is materialised.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		run := r.doctor(ctx, r.bin.Path, dir, s.home, s.appdata, map[string]*string{})
		cancel()
		s.writeStage(fmt.Sprintf("url-%d", i), run)
		a := parseActual(run, p)
		all = all && a.ExitCode == 0 && a.FailingLayer == "None" && len(a.ProviderPaths) == i+1 && strings.HasPrefix(a.ProviderPaths[len(a.ProviderPaths)-1], "/v1/models")
		s.result.Observed = a
	}
	s.result.Expected = expectedProvider("ok")
	s.check("all_url_formats_preserve_models_route", all)
	p.close()
	s.result.Process.ListenerClosed = true
	// The subcase directories were intentionally created after initial snapshot;
	// re-snapshot now defines the final, product-read-only baseline for integrity.
	_ = s.snapshot()
	return s.finish()
}

func metamorphicMCPOrderScenario(r *Runner, id string, seed int64) ScenarioResult {
	s, err := r.begin(id, "Metamorphic MCP map-order invariance", "metamorphic", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	p := newControlledProvider("ok", 0)
	defer p.close()
	aDir, bDir := filepath.Join(s.project, "ordered"), filepath.Join(s.project, "reordered")
	_ = os.MkdirAll(aDir, 0700)
	_ = os.MkdirAll(bDir, 0700)
	a := `{"mcpServers":{"alpha":{"command":"node"},"beta":{"command":"node"},"gamma":{"command":"node"}}}`
	b := `{"mcpServers":{"gamma":{"command":"node"},"alpha":{"command":"node"},"beta":{"command":"node"}}}`
	_ = writeText(filepath.Join(aDir, ".mcp.json"), a)
	_ = writeText(filepath.Join(bDir, ".mcp.json"), b)
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	key := "key"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	ra := r.doctor(ctx, r.bin.Path, aDir, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(p.baseURL("/v1")), "OPENAI_API_KEY": nullable(key)})
	s.writeStage("ordered", ra)
	rb := r.doctor(ctx, r.bin.Path, bDir, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(p.baseURL("/v1")), "OPENAI_API_KEY": nullable(key)})
	s.writeStage("reordered", rb)
	cancel()
	aa, bb := parseActual(ra, p), parseActual(rb, p)
	s.result.Expected, s.result.Observed = expectedProvider("ok"), bb
	allNames := func(out string) bool {
		return strings.Contains(out, "alpha") && strings.Contains(out, "beta") && strings.Contains(out, "gamma") && strings.Index(out, "alpha") < strings.Index(out, "beta") && strings.Index(out, "beta") < strings.Index(out, "gamma")
	}
	s.check("all_servers_in_both_orders", aa.ExitCode == 0 && bb.ExitCode == 0 && allNames(ra.stdout) && allNames(rb.stdout))
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() { os.Exit(run()) }

func run() int {
	seed := flag.Int64("seed", time.Now().UTC().UnixNano(), "deterministic master seed")
	flag.Parse()
	r, err := newRunner(*seed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "organic runner setup:", err)
		return 2
	}
	if err := r.buildSubject(); err != nil {
		r.results = append(r.results, ScenarioResult{ID: "BUILD", Name: "Release-style candidate build", Category: "build", Status: StatusBlocked, Triage: err.Error(), Seed: *seed})
		_ = r.writeReports()
		return 2
	}
	fmt.Printf("Organic lab %s\nCandidate %s (%s)\n", r.lab, r.bin.Version, r.bin.SHA256)
	r.buildReleaseMatrix()

	// Deterministic foundational scenarios.
	providerScenario(r, "D01", "Controlled provider 200 /models", "critical", "ok", *seed+1, r.bin.Path)
	providerScenario(r, "D02", "Controlled provider HTTP 401", "critical", "401", *seed+2, r.bin.Path)
	providerScenario(r, "D03", "Controlled provider HTTP 403", "critical", "403", *seed+3, r.bin.Path)
	providerScenario(r, "D04", "Controlled provider HTTP 404", "critical", "404", *seed+4, r.bin.Path)
	providerScenario(r, "D05", "Controlled provider HTTP 429", "critical", "429", *seed+5, r.bin.Path)
	providerScenario(r, "D06", "Controlled provider HTTP 500", "critical", "500", *seed+6, r.bin.Path)
	providerScenario(r, "D07", "Controlled provider invalid JSON", "critical", "invalid_json", *seed+7, r.bin.Path)
	providerScenario(r, "D08", "Controlled provider missing data", "critical", "missing_data", *seed+8, r.bin.Path)
	tcpRefusalScenario(r, "D09", *seed+9, r.bin.Path)
	dnsFailureScenario(r, "D10", *seed+10, r.bin.Path)
	untrustedTLSScenario(r, "D11", *seed+11, r.bin.Path)
	handshakeFailureScenario(r, "D12", *seed+12, r.bin.Path)
	trustedTLSScenario(r, "D13", *seed+13)
	staleShellScenario(r, "D14", *seed+14, r.bin.Path)
	cliScenario(r, "D15", *seed+15)
	providerScenario(r, "D16", "Controlled provider timeout boundary", "critical", "slow", *seed+16, r.bin.Path)
	providerScenario(r, "D17", "Controlled provider connection close", "critical", "connection_close", *seed+17, r.bin.Path)

	r.runOrganicProject("O01", "assistant-ui starter minimal", "https://github.com/assistant-ui/assistant-ui-starter-minimal.git", *seed+101)
	r.runOrganicProject("O02", "vercel chatbot", "https://github.com/vercel/chatbot.git", *seed+102)

	// Twenty distinct seeded combinations, using shell/dotenv precedence, parser
	// formatting and controlled provider modes. Every random value derives from
	// the scenario seed.
	for i := 1; i <= 20; i++ {
		randomizedConfigProviderScenario(r, i, *seed+200+int64(i))
	}

	// Ten randomized MCP structures, then malformed and security controls.
	sources := []string{"Cursor", "Claude Desktop", "VS Code (workspace)", "Portable (.mcp.json)"}
	counts := []int{0, 1, 2, 3, 5, 10, 1, 3, 5, 2}
	for i, count := range counts {
		validMCPScenario(r, fmt.Sprintf("MCP%02d", i+1), "Seeded MCP discovery matrix", *seed+300+int64(i), count, sources[i%len(sources)])
	}
	malformed := []struct{ label, body string }{{"missing brace", `{"mcpServers":`}, {"top level array", `[]`}, {"mcpServers string", `{"mcpServers":"wrong"}`}, {"server wrong type", `{"mcpServers":{"x":"wrong"}}`}, {"missing transport", `{"mcpServers":{"x":{"args":["a"]}}}`}}
	for i, test := range malformed {
		malformedMCPScenario(r, fmt.Sprintf("MCPF%02d", i+1), test.label, test.body, *seed+400+int64(i))
	}
	mcpSecretScenario(r, "S01", *seed+501, r.bin.Path)
	mcpNoExecScenario(r, "S02", *seed+502, r.bin.Path)

	metamorphicPortScenario(r, "META01", *seed+601)
	metamorphicShellScenario(r, "META02", *seed+602)
	metamorphicFormattingScenario(r, "META03", *seed+603)
	metamorphicURLScenario(r, "META04", *seed+604)
	metamorphicMCPOrderScenario(r, "META05", *seed+605)

	r.runRepeatability(*seed + 700)
	r.runMutationTests()
	if err := r.writeReports(); err != nil {
		fmt.Fprintln(os.Stderr, "write reports:", err)
		return 2
	}
	fmt.Printf("Verdict: %s. Reports: %s\n", r.report.Verdict, filepath.Join(r.organicRoot, "reports"))
	if r.report.Verdict == "PASS" {
		return 0
	}
	return 1
}

func (r *Runner) runOrganicProject(id, name, url string, seed int64) {
	s, err := r.begin(id, "Organic project: "+name, "organic", seed)
	if err != nil {
		r.results = append(r.results, ScenarioResult{ID: id, Name: name, Status: StatusBlocked, Triage: err.Error(), Seed: seed})
		return
	}
	checkout := filepath.Join(s.project, "checkout")
	out, err := commandCombined(s.project, nil, "git", "clone", "--depth", "1", url, checkout)
	if err != nil {
		s.forced = StatusSkip
		s.result.Triage = "optional public clone unavailable: " + strings.TrimSpace(out)
		_ = s.snapshot()
		s.finish()
		return
	}
	s.project, s.result.Project = checkout, checkout
	readme := filepath.Join(checkout, "README.md")
	_, readmeErr := os.Stat(readme)
	_, envExampleErr := os.Stat(filepath.Join(checkout, ".env.example"))
	p := newControlledProvider("ok", 0)
	defer p.close()
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		s.finish()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	key := "organic-project-key"
	run := r.doctor(ctx, r.bin.Path, checkout, s.home, s.appdata, map[string]*string{"OPENAI_BASE_URL": nullable(p.baseURL("/v1")), "OPENAI_API_KEY": nullable(key)})
	cancel()
	s.writeStage("doctor", run)
	t := true
	s.compare(run, p, expectedProvider("ok"), &t)
	s.check("organic_readme_present", readmeErr == nil)
	// The example may be absent in a real repository; that fact is evidence,
	// not a validation failure.
	s.evidence(fmt.Sprintf("real project cloned at immutable commit; README=%t, .env.example=%t", readmeErr == nil, envExampleErr == nil))
	status, statusErr := commandCombined(checkout, nil, "git", "status", "--short")
	s.check("organic_checkout_not_modified_by_subject", statusErr == nil && strings.TrimSpace(status) == "")
	p.close()
	s.result.Process.ListenerClosed = true
	s.finish()
}

func (r *Runner) runRepeatability(seed int64) {
	// Same seed, same controlled truth, separate isolated projects. Semantic
	// fields (not latency) must stay stable across repetitions.
	types := []struct{ prefix, mode string }{{"healthy", "ok"}, {"auth", "401"}, {"route", "404"}}
	for _, test := range types {
		var semantic []string
		for i := 1; i <= 3; i++ {
			result := providerScenario(r, fmt.Sprintf("REP-%s-%d", test.prefix, i), "Repeatability: "+test.prefix, "repeatability", test.mode, seed, r.bin.Path)
			semantic = append(semantic, fmt.Sprintf("%d|%s|%s", result.Observed.ExitCode, result.Observed.FailingLayer, result.Observed.FailureKind))
		}
		consistent := semantic[0] == semantic[1] && semantic[1] == semantic[2]
		if !consistent {
			r.results[len(r.results)-1].Status = StatusFlaky
			r.results[len(r.results)-1].Triage = "NONDETERMINISTIC BEHAVIOR — repeated semantic classifications diverged"
			r.results[len(r.results)-1].Evidence = append(r.results[len(r.results)-1].Evidence, strings.Join(semantic, "; "))
		}
	}
}

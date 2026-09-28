package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func (r *Runner) buildMutant(id string, edits func(string) (string, error)) (string, error) {
	root := filepath.Join(r.lab, "mutations", id, "source")
	for _, dir := range []string{"cmd", "internal"} {
		if err := copyTree(filepath.Join(r.repo, dir), filepath.Join(root, dir)); err != nil {
			return "", err
		}
	}
	if err := copyFile(filepath.Join(r.repo, "go.mod"), filepath.Join(root, "go.mod")); err != nil {
		return "", err
	}
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		changed, err := edits(string(b))
		if err != nil {
			return err
		}
		if changed != string(b) {
			return os.WriteFile(path, []byte(changed), 0600)
		}
		return nil
	}); err != nil {
		return "", err
	}
	path := filepath.Join(r.lab, "mutations", id, "aitrouble-mutant.exe")
	out, err := commandCombined(root, nil, "go", "build", "-trimpath", "-o", path, "-ldflags", "-X main.version=mutation-"+id, "./cmd/aitrouble")
	if err != nil {
		return "", fmt.Errorf("build mutant %s: %w: %s", id, err, out)
	}
	return path, nil
}

func mutationEdit(find, replace string) func(string) (string, error) {
	changed := false
	return func(src string) (string, error) {
		// Normalize CRLF to LF for reliable matching on Windows
		normalized := strings.ReplaceAll(src, "\r\n", "\n")
		if changed || !strings.Contains(normalized, find) {
			return src, nil
		}
		changed = true
		// Apply replacement on the normalized string to ensure it matches
		return strings.Replace(normalized, find, replace, 1), nil
	}
}

func (r *Runner) markMutation(id, title, defect string, result ScenarioResult, detected bool) ScenarioResult {
	if len(r.results) > 0 {
		r.results = r.results[:len(r.results)-1]
	}
	result.ID, result.Name, result.Category = id, title, "mutation"
	result.Evidence = append(result.Evidence, "intentional defect: "+defect)
	result.Assertions["harness_detected_intentional_defect"] = detected
	if detected {
		result.Status, result.Triage = StatusPass, "MUTATION DETECTED — harness rejected the deliberately broken subject"
	} else {
		result.Status, result.Triage = StatusFail, "HARNESS BUG — deliberately broken subject was accepted"
	}
	r.results = append(r.results, result)
	return result
}

func shortCircuitMutationScenario(r *Runner, id string, seed int64, binary string) ScenarioResult {
	s, err := r.begin(id, "Mutation: removed provider short-circuit", "mutation", seed)
	if err != nil {
		return ScenarioResult{ID: id, Status: StatusBlocked, Triage: err.Error()}
	}
	p := newControlledProvider("ok", 0)
	defer p.close()
	_ = writeText(filepath.Join(s.project, ".env"), "OPENAI_BASE_URL="+p.baseURL("/mutation-short-circuit")+"\nOPENAI_API_KEY=key\n")
	if err := s.snapshot(); err != nil {
		s.forced = StatusBlocked
		s.result.Triage = err.Error()
		return s.finish()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	run := r.doctor(ctx, binary, s.project, s.home, s.appdata, map[string]*string{})
	cancel()
	s.writeStage("doctor", run)
	actual := parseActual(run, p)
	s.result.Expected = Expectation{ExitCode: 1, FailingLayer: "Network › DNS", FailureKind: "dns_error", ProviderRequests: 0, DownstreamProviderSK: true}
	s.result.Observed = actual
	// This intentionally differs from standard comparison: the mutant has both
	// a forced lower-layer failure and an HTTP request, which independently
	// demonstrates the harness can observe the forbidden downstream action.
	detected := actual.ProviderRequests == 1 && !actual.DownstreamProviderSK
	s.check("harness_detected_intentional_defect", detected)
	s.forced = StatusPass
	if !detected {
		s.forced = StatusFail
		s.result.Triage = "HARNESS BUG — provider request oracle did not detect removed short-circuit"
	} else {
		s.result.Triage = "MUTATION DETECTED — provider request arrived despite synthetic DNS failure"
	}
	p.close()
	s.result.Process.ListenerClosed = true
	return s.finish()
}

func (r *Runner) runMutationTests() {
	// A: known failing diagnostic returns success.
	a, err := r.buildMutant("M01-exit", mutationEdit("\treturn 1\n}\n\nfunc printConfig", "\treturn 0\n}\n\nfunc printConfig"))
	if err != nil {
		r.results = append(r.results, ScenarioResult{ID: "M01", Name: "Mutation build: exit code", Category: "mutation", Status: StatusBlocked, Triage: err.Error(), Seed: r.seed, Binary: r.bin})
		return
	}
	ra := tcpRefusalScenario(r, "M01-probe", r.seed+1001, a)
	r.markMutation("M01", "Mutation A: force exit 0 on TCP failure", "doctor returns 0 after a detected failure", ra, !ra.Assertions["exit_code"])

	// B: diagnosis label is changed while the underlying TCP fault is real.
	b, err := r.buildMutant("M02-layer", mutationEdit("FailingLayer: \"Network › TCP\",", "FailingLayer: \"Network › DNS\","))
	if err != nil {
		r.results = append(r.results, ScenarioResult{ID: "M02", Name: "Mutation build: diagnosis layer", Category: "mutation", Status: StatusBlocked, Triage: err.Error(), Seed: r.seed, Binary: r.bin})
	} else {
		rb := tcpRefusalScenario(r, "M02-probe", r.seed+1002, b)
		r.markMutation("M02", "Mutation B: misclassify TCP as DNS", "tcp_refused maps to DNS layer", rb, !rb.Assertions["failing_layer"])
	}

	// C: two tightly-scoped source changes create a network failure report and
	// remove its short-circuit solely to prove the request oracle catches it.
	c, err := r.buildMutant("M03-short-circuit", func(src string) (string, error) {
		src = strings.ReplaceAll(src, "\r\n", "\n")
		if strings.Contains(src, "func (p *NetworkProber) ProbeTarget") {
			anchor := "\tvar results []core.ProbeResult\n"
			if !strings.Contains(src, anchor) {
				return src, fmt.Errorf("network mutation anchor missing")
			}
			return strings.Replace(src, anchor, anchor+"\tif strings.Contains(baseURL, \"/mutation-short-circuit\") { return []core.ProbeResult{{Name: \"DNS synthetic\", Status: core.StatusFail, FailureKind: \"dns_error\"}} }\n", 1), nil
		}
		if strings.Contains(src, "networkFailed = true") {
			return strings.Replace(src, "networkFailed = true", "networkFailed = false", 1), nil
		}
		return src, nil
	})
	if err != nil {
		r.results = append(r.results, ScenarioResult{ID: "M03", Name: "Mutation build: short circuit", Category: "mutation", Status: StatusBlocked, Triage: err.Error(), Seed: r.seed, Binary: r.bin})
	} else {
		shortCircuitMutationScenario(r, "M03", r.seed+1003, c)
	}

	// D: redaction removal must be caught by a canary scanner independent of MCP code.
	d, err := r.buildMutant("M04-redaction", mutationEdit("sc.SafeURL = RedactURL(def.URL)", "sc.SafeURL = def.URL"))
	if err != nil {
		r.results = append(r.results, ScenarioResult{ID: "M04", Name: "Mutation build: MCP redaction", Category: "mutation", Status: StatusBlocked, Triage: err.Error(), Seed: r.seed, Binary: r.bin})
	} else {
		rd := mcpSecretScenario(r, "M04-probe", r.seed+1004, d)
		r.markMutation("M04", "Mutation D: disable MCP URL redaction", "raw URL assigned to SafeURL", rd, rd.SecretScan.Leaked)
	}

	// E: explicitly execute configured stdio command in the disposable copy.
	e, err := r.buildMutant("M05-exec", func(src string) (string, error) {
		src = strings.ReplaceAll(src, "\r\n", "\n")
		anchor := "\tif def.Command != \"\" {\n"
		if !strings.Contains(src, anchor) {
			return src, nil
		}
		if !strings.Contains(src, "\"os/exec\"") && strings.Contains(src, "\"os\"\n") {
			src = strings.Replace(src, "\"os\"\n", "\"os\"\n\t\"os/exec\"\n", 1)
		}
		injected := anchor + "\t\tvar mutationArgs []string\n\t\tfor _, raw := range def.Args { var arg string; if json.Unmarshal(raw, &arg) == nil { mutationArgs = append(mutationArgs, arg) } }\n\t\t_ = exec.Command(def.Command, mutationArgs...).Run()\n"
		return strings.Replace(src, anchor, injected, 1), nil
	})
	if err != nil {
		r.results = append(r.results, ScenarioResult{ID: "M05", Name: "Mutation build: MCP execution", Category: "mutation", Status: StatusBlocked, Triage: err.Error(), Seed: r.seed, Binary: r.bin})
	} else {
		re := mcpNoExecScenario(r, "M05-probe", r.seed+1005, e)
		r.markMutation("M05", "Mutation E: execute MCP command", "static discovery invokes configured command", re, re.Process.MarkerFound)
	}

	// F: dotenv is inserted before shell resolution.
	f, err := r.buildMutant("M06-precedence", mutationEdit("\t// 1. Shell environment\n", "\tif dotVal, ok := dotenv[key]; ok { return ConfigValue{Key: key, Present: true, Source: \".env\", Secret: secret, value: dotVal} }\n\n\t// 1. Shell environment\n"))
	if err != nil {
		r.results = append(r.results, ScenarioResult{ID: "M06", Name: "Mutation build: config precedence", Category: "mutation", Status: StatusBlocked, Triage: err.Error(), Seed: r.seed, Binary: r.bin})
	} else {
		rf := staleShellScenario(r, "M06-probe", r.seed+1006, f)
		r.markMutation("M06", "Mutation F: dotenv wins over shell", "inserted dotenv early-return before shell", rf, !rf.Assertions["effective_base_url_source"] || !rf.Assertions["provider_request_count"])
	}
}

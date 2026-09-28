package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (r *Runner) buildReleaseMatrix() {
	pairs := [][2]string{{"windows", "amd64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}}
	for _, pair := range pairs {
		goos, goarch := pair[0], pair[1]
		name := fmt.Sprintf("aitrouble-%s-%s", goos, goarch)
		if goos == "windows" {
			name += ".exe"
		}
		path := filepath.Join(r.lab, "bin", name)
		env := append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
		out, err := commandCombined(r.repo, env, "go", "build", "-trimpath", "-o", path, "-ldflags", "-s -w -X main.version=organic-"+r.bin.Describe, "./cmd/aitrouble")
		entry := ReleaseBuild{GOOS: goos, GOARCH: goarch, Status: StatusPass, Artifact: path}
		if err != nil {
			entry.Status, entry.Detail = StatusFail, strings.TrimSpace(out)+": "+err.Error()
		} else {
			entry.SHA256, _ = sha256File(path)
			if goos != r.bin.GOOS || goarch != r.bin.GOARCH {
				entry.Detail = "BUILD VERIFIED only; not executed on this host"
			} else {
				entry.Detail = "RUNTIME VERIFIED by organic scenarios"
			}
		}
		r.report.ReleaseBuilds = append(r.report.ReleaseBuilds, entry)
	}
}

func (r *Runner) writeReports() error {
	r.report.Results = r.results
	r.report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.report.Totals = map[string]int{}
	for _, result := range r.results {
		r.report.Totals[string(result.Status)]++
	}
	r.report.Verdict = "PASS"
	if r.report.Totals[string(StatusFail)] > 0 {
		r.report.Verdict = "FAIL"
	} else if r.report.Totals[string(StatusBlocked)] > 0 || r.report.Totals[string(StatusFlaky)] > 0 {
		r.report.Verdict = "PARTIAL"
	}
	r.report.Notes = append(r.report.Notes,
		"Existing pre-run organic artifacts were not used as validation evidence.",
		"All test-owned run data is preserved under the unique lab directory; no cleanup deletion is performed.",
		"External project cloning is optional evidence. Controlled local infrastructure is the oracle for deterministic network/provider assertions.",
	)
	b, err := json.MarshalIndent(r.report, "", "  ")
	if err != nil {
		return err
	}
	seedIndex := make([]map[string]any, 0, len(r.results))
	for _, result := range r.results {
		seedIndex = append(seedIndex, map[string]any{"id": result.ID, "name": result.Name, "seed": result.Seed, "status": result.Status, "artifact_dir": result.ArtifactDir})
	}
	seedBytes, _ := json.MarshalIndent(map[string]any{"master_seed": r.seed, "run_id": r.runID, "scenarios": seedIndex}, "", "  ")
	md := r.markdownReport()
	failures := r.failuresReport()
	mutations := r.mutationReport()
	for _, dir := range []string{filepath.Join(r.lab, "reports"), filepath.Join(r.organicRoot, "reports")} {
		if err := writeText(filepath.Join(dir, "organic-test-summary.json"), string(b)); err != nil {
			return err
		}
		if err := writeText(filepath.Join(dir, "ORGANIC_TEST_REPORT.md"), md); err != nil {
			return err
		}
		if err := writeText(filepath.Join(dir, "FAILURES.md"), failures); err != nil {
			return err
		}
		if err := writeText(filepath.Join(dir, "MUTATION_TEST_REPORT.md"), mutations); err != nil {
			return err
		}
		if err := writeText(filepath.Join(dir, "SEED_INDEX.json"), string(seedBytes)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) markdownReport() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Organic Validation Report\n\n**Verdict: %s**\n\n", r.report.Verdict)
	fmt.Fprintf(&b, "Run: `%s`  \nMaster seed: `%d`  \nLab: `%s`\n\n", r.runID, r.seed, r.lab)
	b.WriteString("## Test subject\n\n")
	fmt.Fprintf(&b, "- Binary: `%s`\n- SHA-256: `%s`\n- Commit: `%s` (`%s`)\n- Version: `%s`\n- Runtime: `%s/%s`, `%s`\n\n", r.bin.Path, r.bin.SHA256, r.bin.Commit, r.bin.Describe, r.bin.Version, r.bin.GOOS, r.bin.GOARCH, r.bin.GoVersion)
	b.WriteString("## Counts\n\n")
	keys := make([]string, 0, len(r.report.Totals))
	for key := range r.report.Totals {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "- %s: %d\n", key, r.report.Totals[key])
	}
	b.WriteString("\n## Release-style builds\n\n| Target | Status | Scope |\n|---|---|---|\n")
	for _, build := range r.report.ReleaseBuilds {
		fmt.Fprintf(&b, "| %s/%s | %s | %s |\n", build.GOOS, build.GOARCH, build.Status, build.Detail)
	}
	for _, result := range r.results {
		fmt.Fprintf(&b, "\n## %s — %s\n\n", result.ID, result.Name)
		fmt.Fprintf(&b, "Status: **%s**  \nCategory: %s  \nSeed: `%d`  \nArtifact: `%s`\n\n", result.Status, result.Category, result.Seed, result.ArtifactDir)
		if result.Triage != "" {
			fmt.Fprintf(&b, "Triage: %s\n\n", result.Triage)
		}
		fmt.Fprintf(&b, "Precondition: %s  \nFault injection: %s\n\n", nonEmpty(result.Precondition, "test-owned isolated fixture"), nonEmpty(result.FaultInjection, "none"))
		fmt.Fprintf(&b, "Expected: exit `%d`, layer `%s`, kind `%s`, provider requests `%d`.  \n", result.Expected.ExitCode, printable(result.Expected.FailingLayer), printable(result.Expected.FailureKind), result.Expected.ProviderRequests)
		fmt.Fprintf(&b, "Observed: exit `%d`, layer `%s`, kind `%s`, provider requests `%d` (`%s`).\n\n", result.Observed.ExitCode, printable(result.Observed.FailingLayer), printable(result.Observed.FailureKind), result.Observed.ProviderRequests, result.Observed.ProviderRequest)
		b.WriteString("Assertions: ")
		assertionKeys := make([]string, 0, len(result.Assertions))
		for k := range result.Assertions {
			assertionKeys = append(assertionKeys, k)
		}
		sort.Strings(assertionKeys)
		for i, key := range assertionKeys {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s=%t", key, result.Assertions[key])
		}
		b.WriteString(".\n\n")
		fmt.Fprintf(&b, "Security: canary leak=%t; filesystem unchanged=%t; environment restored=%t; unexpected MCP process=%t.  \n", result.SecretScan.Leaked, result.Filesystem.Unchanged, result.Environment.Restored, result.Process.UnexpectedChild)
		fmt.Fprintf(&b, "Duration: %d ms.\n", result.DurationMS)
		if len(result.Evidence) > 0 {
			b.WriteString("\nEvidence:\n\n")
			for _, e := range result.Evidence {
				fmt.Fprintf(&b, "- %s\n", e)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (r *Runner) failuresReport() string {
	var b strings.Builder
	b.WriteString("# Failures and limitations\n\n")
	count := 0
	for _, result := range r.results {
		if result.Status == StatusPass || result.Status == StatusSkip {
			continue
		}
		count++
		fmt.Fprintf(&b, "## %s — %s (%s)\n\nTriage: %s\n\nExpected `%s/%s`; observed `%s/%s`. Artifact: `%s`.\n\n", result.ID, result.Name, result.Status, nonEmpty(result.Triage, "unclassified"), printable(result.Expected.FailingLayer), printable(result.Expected.FailureKind), printable(result.Observed.FailingLayer), printable(result.Observed.FailureKind), result.ArtifactDir)
	}
	if count == 0 {
		b.WriteString("No FAIL, BLOCKED, or FLAKY scenarios were observed.\n")
	}
	return b.String()
}

func (r *Runner) mutationReport() string {
	var b strings.Builder
	b.WriteString("# Mutation Test Report\n\n")
	count := 0
	for _, result := range r.results {
		if result.Category != "mutation" {
			continue
		}
		count++
		fmt.Fprintf(&b, "## %s — %s\n\nStatus: **%s**  \n%s\n\n", result.ID, result.Name, result.Status, result.Triage)
	}
	if count == 0 {
		b.WriteString("Mutation suite did not execute.\n")
	}
	return b.String()
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func printable(value string) string {
	if value == "" {
		return "(none emitted)"
	}
	return value
}

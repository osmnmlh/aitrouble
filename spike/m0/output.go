package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Report bundles all probe results and the final diagnosis for output.
type Report struct {
	Config   map[string]ConfigValue
	DNS      *ProbeResult
	TCP      *ProbeResult
	TLS      *ProbeResult
	Provider *ProbeResult
	Diag     Diagnosis
}

// PrintTerminal writes a plain-text diagnostic report to w.
// No TUI, no colour codes — plain terminal output only for M0.
func PrintTerminal(w io.Writer, r Report) {
	fmt.Fprintln(w, "aitrouble M0 spike")
	fmt.Fprintln(w)

	// ── Configuration ─────────────────────────────────────────────────────────
	fmt.Fprintln(w, "Configuration")
	for _, key := range ConfigKeys {
		cv, ok := r.Config[key]
		if !ok || !cv.Present {
			fmt.Fprintf(w, "  ✗ %-22s absent\n", key)
			continue
		}
		if cv.Secret {
			fmt.Fprintf(w, "  ✓ %-22s present=true  source=%s\n", cv.Key, cv.Source)
		} else {
			fmt.Fprintf(w, "  ✓ %-22s value=%-40s source=%s\n", cv.Key, cv.DisplayValue(), cv.Source)
		}
	}
	fmt.Fprintln(w)

	// ── Network ───────────────────────────────────────────────────────────────
	fmt.Fprintln(w, "Network")
	printProbe(w, r.DNS, "DNS")
	printProbe(w, r.TCP, "TCP")
	printProbe(w, r.TLS, "TLS")
	fmt.Fprintln(w)

	// ── Provider ──────────────────────────────────────────────────────────────
	fmt.Fprintln(w, "Provider")
	printProbe(w, r.Provider, "authentication")
	fmt.Fprintln(w)

	// ── Diagnosis ─────────────────────────────────────────────────────────────
	sep := strings.Repeat("─", 40)
	fmt.Fprintln(w, sep)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "PRIMARY DIAGNOSIS")
	fmt.Fprintln(w)
	fmt.Fprintln(w, r.Diag.Summary)

	if len(r.Diag.Evidence) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Evidence:")
		for _, e := range r.Diag.Evidence {
			fmt.Fprintf(w, "  %s\n", e)
		}
	}

	if len(r.Diag.Candidates) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Likely causes:")
		for _, c := range r.Diag.Candidates {
			fmt.Fprintf(w, "  → %s\n", c.Reason)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Confidence: %s\n", strings.ToUpper(r.Diag.Candidates[0].Confidence))
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Secrets: REDACTED")

	if r.Diag.FixHint != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Next step: %s\n", r.Diag.FixHint)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, sep)
}

func printProbe(w io.Writer, r *ProbeResult, label string) {
	if r == nil {
		fmt.Fprintf(w, "  – %s (skipped)\n", label)
		return
	}
	switch r.Status {
	case StatusPass:
		lat := ""
		if r.Latency > 0 {
			lat = " (" + r.Latency.Round(time.Millisecond).String() + ")"
		}
		fmt.Fprintf(w, "  ✓ %s%s\n", label, lat)
	case StatusFail:
		suffix := ""
		if len(r.Evidence) > 0 {
			suffix = " — " + r.Evidence[0]
		}
		fmt.Fprintf(w, "  ✗ %s%s\n", label, suffix)
	default:
		fmt.Fprintf(w, "  – %s (skipped)\n", label)
	}
}

// ── JSON output ───────────────────────────────────────────────────────────────

// JSONOutput is the stable M0 JSON schema (schema_version: 1).
// This schema must not change during M0. It will be redesigned for production.
type JSONOutput struct {
	SchemaVersion int           `json:"schema_version"`
	Checks        []JSONCheck   `json:"checks"`
	Diagnosis     JSONDiagnosis `json:"diagnosis"`
}

// JSONCheck is a single check entry in the JSON output.
type JSONCheck struct {
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	FailureKind string   `json:"failure_kind,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
}

// JSONDiagnosis is the diagnosis block in the JSON output.
type JSONDiagnosis struct {
	Code       string `json:"code"`
	Summary    string `json:"summary"`
	Confidence string `json:"confidence"`
	FixHint    string `json:"fix_hint,omitempty"`
}

// PrintJSON writes a structured JSON report to w.
// Secrets are never included; see ConfigValue.DisplayValue().
func PrintJSON(w io.Writer, r Report) error {
	out := JSONOutput{
		SchemaVersion: 1,
		Checks:        jsonChecks(r),
		Diagnosis:     jsonDiagnosis(r.Diag),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func jsonChecks(r Report) []JSONCheck {
	var checks []JSONCheck

	for _, key := range ConfigKeys {
		cv, ok := r.Config[key]
		status := "fail"
		var ev []string
		if ok && cv.Present {
			status = "pass"
			if cv.Secret {
				ev = []string{"present=true source=" + cv.Source}
			} else {
				ev = []string{"value=" + cv.DisplayValue() + " source=" + cv.Source}
			}
		}
		checks = append(checks, JSONCheck{Name: "config:" + key, Status: status, Evidence: ev})
	}

	for _, pr := range []*ProbeResult{r.DNS, r.TCP, r.TLS, r.Provider} {
		if pr == nil {
			continue
		}
		checks = append(checks, JSONCheck{
			Name:        pr.Name,
			Status:      string(pr.Status),
			FailureKind: pr.FailureKind,
			Evidence:    pr.Evidence,
		})
	}
	return checks
}

func jsonDiagnosis(d Diagnosis) JSONDiagnosis {
	conf := ""
	if len(d.Candidates) > 0 {
		conf = d.Candidates[0].Confidence
	}
	return JSONDiagnosis{
		Code:       d.Code,
		Summary:    d.Summary,
		Confidence: conf,
		FixHint:    d.FixHint,
	}
}

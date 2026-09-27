package doctor

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/osmnmlh/aitrouble/internal/core"
	"github.com/osmnmlh/aitrouble/internal/diagnosis"
	"github.com/osmnmlh/aitrouble/internal/network"
	"github.com/osmnmlh/aitrouble/internal/provider"
)

// NetworkProber is the interface satisfied by network.NetworkProber.
type NetworkProber interface {
	ProbeTarget(ctx context.Context, baseURL string) []core.ProbeResult
}

// ProviderProber is the interface satisfied by provider.ProviderProber.
type ProviderProber interface {
	ProbeModels(ctx context.Context, cfg core.EffectiveConfig) core.ProbeResult
}

// deps holds injectable dependencies for testability.
type deps struct {
	net  NetworkProber
	prov ProviderProber
}

// Run orchestrates the doctor command using real production probers.
// It returns an exit code (0 for pass/healthy, 1 for failure detected, 2 for usage error).
func Run(ctx context.Context, envFile string, stdout, stderr io.Writer) int {
	d := deps{
		net:  network.NewDefaultProber(10 * time.Second),
		prov: provider.NewProviderProber(nil, 30*time.Second),
	}
	return runWithDeps(ctx, envFile, stdout, stderr, d)
}

// runWithDeps is the testable implementation — deps are injected.
func runWithDeps(ctx context.Context, envFile string, stdout, stderr io.Writer, d deps) int {
	fmt.Fprintln(stdout, "aitrouble doctor")
	fmt.Fprintln(stdout, "Find where your AI integration breaks.")
	fmt.Fprintln(stdout, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Resolve Effective Configuration
	fmt.Fprintln(stdout, "\n[1/3] Effective Configuration")
	cfg, err := core.ResolveEffectiveConfig(envFile)
	if err != nil {
		fmt.Fprintf(stderr, "  ✗  Failed to resolve configuration: %v\n", err)
		return 1
	}

	printConfig(stdout, cfg)

	baseURLVal, ok := cfg.Get("OPENAI_BASE_URL")
	if !ok || baseURLVal.String() == "" {
		fmt.Fprintln(stderr, "  ✗  Missing OPENAI_BASE_URL configuration.")
		return 1
	}
	baseURL := baseURLVal.String()

	// 2. Network Probes
	fmt.Fprintln(stdout, "\n[2/3] Network Probes")
	netResults := d.net.ProbeTarget(ctx, baseURL)
	printResults(stdout, netResults)

	// Check if network failed
	networkFailed := false
	for _, res := range netResults {
		if res.Status == core.StatusFail {
			networkFailed = true
			break
		}
	}

	// 3. Provider Probe
	fmt.Fprintln(stdout, "\n[3/3] Provider Probe")
	var provResult core.ProbeResult

	if networkFailed {
		provResult = core.ProbeResult{
			Name:     "OpenAI /models",
			Status:   core.StatusSkip,
			Evidence: []string{"skipped due to network failure"},
		}
		printResults(stdout, []core.ProbeResult{provResult})
	} else {
		provResult = d.prov.ProbeModels(ctx, cfg)
		printResults(stdout, []core.ProbeResult{provResult})
	}

	// 4. Correlate Results
	allResults := append(netResults, provResult)
	diag := diagnosis.Correlate(allResults)

	fmt.Fprintln(stdout, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if diag.FailingLayer == "None" {
		fmt.Fprintln(stdout, " DIAGNOSIS  All tested components appear healthy.")
		fmt.Fprintln(stdout, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		return 0
	}

	fmt.Fprintf(stdout, " DIAGNOSIS  The chain breaks at: %s\n", diag.FailingLayer)
	fmt.Fprintf(stdout, " SUMMARY    %s\n", diag.Summary)
	fmt.Fprintf(stdout, " FIX        %s\n", diag.FixHint)
	fmt.Fprintln(stdout, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return 1
}

func printConfig(w io.Writer, cfg core.EffectiveConfig) {
	printConfigValue(w, cfg, "OPENAI_API_KEY")
	printConfigValue(w, cfg, "OPENAI_BASE_URL")
}

func printConfigValue(w io.Writer, cfg core.EffectiveConfig, key string) {
	val, ok := cfg.Get(key)
	if !ok || !val.Present {
		fmt.Fprintf(w, "  ⚠  %-16s (not set)\n", key)
		return
	}
	fmt.Fprintf(w, "  ✓  %-16s %-15s   source: %s\n", key, val.String(), val.Source)
}

func printResults(w io.Writer, results []core.ProbeResult) {
	for _, res := range results {
		icon := "–"
		if res.Status == core.StatusPass {
			icon = "✓"
		} else if res.Status == core.StatusFail {
			icon = "✗"
		}

		line := fmt.Sprintf("  %s  %-40s", icon, res.Name)
		if res.Latency > 0 {
			line += fmt.Sprintf(" (%v)", res.Latency.Round(time.Millisecond))
		}
		if res.FailureKind != "" {
			line += fmt.Sprintf(" [%s]", res.FailureKind)
		}
		fmt.Fprintln(w, line)

		// Print evidence if any
		if len(res.Evidence) > 0 {
			ev := diagnosis.FormatEvidence(res.Evidence)
			if ev != "" {
				fmt.Fprintf(w, "     Evidence: %s\n", ev)
			}
		}
	}
}

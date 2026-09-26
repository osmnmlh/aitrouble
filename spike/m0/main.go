// aitrouble M0 spike — cross-layer AI integration troubleshooter.
//
// Usage:
//
//	aitrouble-spike [flags]
//
// Flags:
//
//	--env-file string   path to .env file (default ".env"; "" to skip)
//	--timeout duration  per-probe deadline (default 10s)
//	--json              output JSON (schema_version: 1) instead of plain text
//	--insecure          skip TLS certificate verification (TESTING ONLY)
//
// The API key is read from the OPENAI_API_KEY environment variable or .env
// file. It is NEVER accepted as a command-line flag.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

func main() {
	var (
		envFile  = flag.String("env-file", ".env", `path to .env file ("" to skip)`)
		timeout  = flag.Duration("timeout", 10*time.Second, "per-probe deadline")
		jsonOut  = flag.Bool("json", false, "output JSON (schema_version: 1)")
		insecure = flag.Bool("insecure", false, "skip TLS certificate verification (TESTING ONLY)")
	)
	flag.Parse()

	ctx := context.Background()

	// ── 1. Resolve Effective Configuration ────────────────────────────────────
	cfg, err := ResolveEffectiveConfig(ConfigKeys, *envFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// ── 2. Determine target from OPENAI_BASE_URL ───────────────────────────────
	baseURLCV := cfg["OPENAI_BASE_URL"]
	if !baseURLCV.Present || baseURLCV.RawValue() == "" {
		// No base URL — run config-only diagnosis and exit.
		ps := ProbeSet{Config: cfg}
		diag := Correlate(ps)
		emitReport(Report{Config: cfg, Diag: diag}, *jsonOut)
		exitCode(diag)
	}

	baseURL := baseURLCV.RawValue()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid OPENAI_BASE_URL %q: %v\n", baseURL, err)
		os.Exit(1)
	}

	host := parsed.Hostname()
	port := hostPort(parsed)
	scheme := parsed.Scheme

	// ── 3. Network probes ─────────────────────────────────────────────────────
	prober := DefaultNetworkProber(*timeout)
	if *insecure {
		prober.TLSConfig = &tls.Config{ //nolint:gosec // intentional for testing
			InsecureSkipVerify: true,
			ServerName:         host,
		}
	}

	dnsResult := prober.ProbeDNS(ctx, host)

	var tcpResult, tlsResult *ProbeResult
	if dnsResult.Status == StatusPass {
		r := prober.ProbeTCP(ctx, host, port)
		tcpResult = &r

		if r.Status == StatusPass && scheme == "https" {
			t := prober.ProbeTLS(ctx, host, port)
			tlsResult = &t
		}
	}

	// ── 4. Provider probe (skipped when network is unhealthy) ─────────────────
	var providerResult *ProbeResult

	networkOK := dnsResult.Status == StatusPass &&
		(tcpResult == nil || tcpResult.Status == StatusPass)

	apiKeyCV := cfg["OPENAI_API_KEY"]
	if networkOK && apiKeyCV.Present && apiKeyCV.RawValue() != "" {
		pp := NewProviderProber(apiKeyCV.RawValue(), nil)
		pr := pp.ProbeModels(ctx, baseURL)
		providerResult = &pr
	}

	// ── 5. Correlate and report ────────────────────────────────────────────────
	ps := ProbeSet{
		Config:   cfg,
		DNS:      &dnsResult,
		TCP:      tcpResult,
		TLS:      tlsResult,
		Provider: providerResult,
	}
	diag := Correlate(ps)
	report := Report{
		Config:   cfg,
		DNS:      ps.DNS,
		TCP:      ps.TCP,
		TLS:      ps.TLS,
		Provider: ps.Provider,
		Diag:     diag,
	}

	emitReport(report, *jsonOut)
	exitCode(diag)
}

func emitReport(r Report, asJSON bool) {
	if asJSON {
		if err := PrintJSON(os.Stdout, r); err != nil {
			fmt.Fprintf(os.Stderr, "JSON output error: %v\n", err)
			os.Exit(1)
		}
	} else {
		PrintTerminal(os.Stdout, r)
	}
}

func exitCode(d Diagnosis) {
	if d.Code == DiagHealthy {
		os.Exit(0)
	}
	os.Exit(1)
}

// hostPort extracts the port number from a parsed URL, defaulting to 443
// for https and 80 for http.
func hostPort(u *url.URL) int {
	if portStr := u.Port(); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			return p
		}
	}
	switch u.Scheme {
	case "http":
		return 80
	default:
		return 443
	}
}

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// DialContextFunc dials a network connection.
// It has the same signature as net.Dialer.DialContext.
//
// Tests inject deterministic fake implementations to avoid real network calls
// and to produce specific failure modes without OS dependencies.
type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

// DNSLookupFunc resolves a hostname to IP addresses.
// Tests inject deterministic fake implementations.
type DNSLookupFunc func(ctx context.Context, host string) ([]string, error)

// NetworkProber runs deterministic network probes.
// All external dependencies are injectable for hermetic testing.
type NetworkProber struct {
	Dial      DialContextFunc // injectable TCP dialer
	DNSLookup DNSLookupFunc  // injectable DNS resolver
	TLSConfig *tls.Config    // nil → secure defaults (hostname verified)
	Timeout   time.Duration  // per-probe deadline
}

// DefaultNetworkProber wires a NetworkProber to the real OS networking stack.
func DefaultNetworkProber(timeout time.Duration) *NetworkProber {
	d := &net.Dialer{}
	return &NetworkProber{
		Dial: d.DialContext,
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return net.DefaultResolver.LookupHost(ctx, host)
		},
		Timeout: timeout,
	}
}

// ProbeDNS resolves host and returns a ProbeResult.
func (p *NetworkProber) ProbeDNS(ctx context.Context, host string) ProbeResult {
	lookCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	start := time.Now()
	addrs, err := p.DNSLookup(lookCtx, host)
	latency := time.Since(start)

	if err != nil {
		return ProbeResult{
			Name:        "DNS " + host,
			Status:      StatusFail,
			FailureKind: "dns_error",
			Latency:     latency,
			Evidence:    []string{safeDNSSummary(err)},
		}
	}

	ev := []string{fmt.Sprintf("resolved %d address(es)", len(addrs))}
	if len(addrs) > 0 {
		ev = append(ev, "first: "+addrs[0])
	}
	return ProbeResult{
		Name:     "DNS " + host,
		Status:   StatusPass,
		Latency:  latency,
		Evidence: ev,
	}
}

// ProbeTCP attempts a TCP connection via the injectable Dial function.
// The Dial function controls all network behaviour; tests inject fakes.
func (p *NetworkProber) ProbeTCP(ctx context.Context, host string, port int) ProbeResult {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	start := time.Now()
	conn, err := p.Dial(dialCtx, "tcp", addr)
	latency := time.Since(start)

	if err != nil {
		return ProbeResult{
			Name:        "TCP " + addr,
			Status:      StatusFail,
			FailureKind: classifyDialError(err),
			Latency:     latency,
			Evidence:    []string{safeDialSummary(err)},
		}
	}
	conn.Close()
	return ProbeResult{
		Name:    "TCP " + addr,
		Status:  StatusPass,
		Latency: latency,
	}
}

// ProbeTLS establishes a TCP connection via the injectable Dial function and
// then performs a TLS handshake on top.  This allows TLS probe tests to inject
// a fake TCP layer while still exercising the real crypto/tls code path.
func (p *NetworkProber) ProbeTLS(ctx context.Context, host string, port int) ProbeResult {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	tlsCfg := p.TLSConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	} else {
		tlsCfg = tlsCfg.Clone()
		if tlsCfg.ServerName == "" {
			tlsCfg.ServerName = host
		}
	}

	dialCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	start := time.Now()
	rawConn, err := p.Dial(dialCtx, "tcp", addr)
	if err != nil {
		return ProbeResult{
			Name:        "TLS " + addr,
			Status:      StatusFail,
			FailureKind: classifyDialError(err),
			Latency:     time.Since(start),
			Evidence:    []string{safeDialSummary(err)},
		}
	}

	tlsConn := tls.Client(rawConn, tlsCfg)
	if err := tlsConn.HandshakeContext(dialCtx); err != nil {
		rawConn.Close()
		return ProbeResult{
			Name:        "TLS " + addr,
			Status:      StatusFail,
			FailureKind: classifyTLSError(err),
			Latency:     time.Since(start),
			Evidence:    []string{safeTLSSummary(err)},
		}
	}

	state := tlsConn.ConnectionState()
	tlsConn.Close()
	return ProbeResult{
		Name:    "TLS " + addr,
		Status:  StatusPass,
		Latency: time.Since(start),
		Evidence: []string{
			"version: " + tlsVersionString(state.Version),
		},
	}
}

// ── Error classification ──────────────────────────────────────────────────────

// classifyDialError returns a normalised, machine-readable failure kind.
// Tests MUST NOT assert against OS-specific error strings; they use fake
// dialers that return pre-classified errors instead.
func classifyDialError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "tcp_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "tcp_cancelled"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns_error"
	}
	if isConnectionRefused(err) {
		return "tcp_refused"
	}
	return "tcp_error"
}

// isConnectionRefused detects an active connection-refused error.
// Uses string matching as a cross-platform fallback because syscall error
// codes (ECONNREFUSED / WSAECONNREFUSED) are not safely portable via
// errors.As across all supported Go targets in this spike.
//
// Limitation: string matching is fragile; production code should use
// platform-specific build tags or errors.As with syscall.Errno.
func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "connection refused") || // Linux, macOS
		strings.Contains(s, "actively refused") || // Windows (some versions)
		strings.Contains(s, "No connection could be made") // Windows (WSAECONNREFUSED)
}

func classifyTLSError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "tcp_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "tcp_cancelled"
	}
	s := err.Error()
	if strings.Contains(s, "certificate") || strings.Contains(s, "x509") {
		return "tls_cert_error"
	}
	return "tls_error"
}

// ── Safe error summaries (no secrets, no raw OS strings) ─────────────────────

func safeDNSSummary(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return "hostname not found"
		}
		if dnsErr.IsTimeout {
			return "DNS lookup timed out"
		}
		return "DNS error"
	}
	return "DNS lookup failed"
}

func safeDialSummary(err error) string {
	switch classifyDialError(err) {
	case "tcp_refused":
		return "connection actively refused by target"
	case "tcp_timeout":
		return "connection attempt timed out"
	case "tcp_cancelled":
		return "connection attempt cancelled"
	case "dns_error":
		return safeDNSSummary(err)
	default:
		return "TCP connection failed"
	}
}

func safeTLSSummary(err error) string {
	switch classifyTLSError(err) {
	case "tcp_timeout":
		return "TLS handshake timed out"
	case "tcp_cancelled":
		return "TLS handshake cancelled"
	case "tls_cert_error":
		return "TLS certificate validation failed"
	default:
		return "TLS handshake failed"
	}
}

func tlsVersionString(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("TLS 0x%04x", v)
	}
}

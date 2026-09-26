package network

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/osmnmlh/aitrouble/internal/core"
)

// DialContextFunc dials a network connection.
type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

// DNSLookupFunc resolves a hostname to IP addresses.
type DNSLookupFunc func(ctx context.Context, host string) ([]string, error)

// NetworkProber runs deterministic network probes.
type NetworkProber struct {
	Dial      DialContextFunc
	DNSLookup DNSLookupFunc
	TLSConfig *tls.Config
	Timeout   time.Duration
}

// NewDefaultProber wires a NetworkProber to the real OS networking stack.
func NewDefaultProber(timeout time.Duration) *NetworkProber {
	d := &net.Dialer{}
	return &NetworkProber{
		Dial: d.DialContext,
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return net.DefaultResolver.LookupHost(ctx, host)
		},
		Timeout: timeout,
	}
}

// ParseTarget parses a base URL and extracts scheme, host, and port.
// Only http and https schemes are supported.
func ParseTarget(baseURL string) (scheme, host string, port int, err error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid URL: %w", err)
	}

	scheme = u.Scheme
	if scheme != "http" && scheme != "https" {
		return "", "", 0, errors.New("unsupported URL scheme")
	}

	host = u.Hostname()
	if host == "" {
		return "", "", 0, errors.New("missing hostname")
	}

	portStr := u.Port()
	if portStr == "" {
		if scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	} else {
		p, parseErr := strconv.Atoi(portStr)
		if parseErr != nil {
			return "", "", 0, fmt.Errorf("invalid port: %w", parseErr)
		}
		port = p
	}

	return scheme, host, port, nil
}

// ProbeDNS resolves host and returns a ProbeResult.
func (p *NetworkProber) ProbeDNS(ctx context.Context, host string) core.ProbeResult {
	lookCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	start := time.Now()
	addrs, err := p.DNSLookup(lookCtx, host)
	latency := time.Since(start)

	if err != nil {
		return core.ProbeResult{
			Name:        "DNS " + host,
			Status:      core.StatusFail,
			FailureKind: classifyDNSError(err),
			Latency:     latency,
			Evidence:    []string{safeDNSSummary(err)},
		}
	}

	ev := []string{fmt.Sprintf("resolved %d address(es)", len(addrs))}
	if len(addrs) > 0 {
		ev = append(ev, "first: "+addrs[0])
	}
	return core.ProbeResult{
		Name:     "DNS " + host,
		Status:   core.StatusPass,
		Latency:  latency,
		Evidence: ev,
	}
}

// ProbeTCP attempts a TCP connection.
func (p *NetworkProber) ProbeTCP(ctx context.Context, host string, port int) core.ProbeResult {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	start := time.Now()
	conn, err := p.Dial(dialCtx, "tcp", addr)
	latency := time.Since(start)

	if err != nil {
		return core.ProbeResult{
			Name:        "TCP " + addr,
			Status:      core.StatusFail,
			FailureKind: classifyDialError(err),
			Latency:     latency,
			Evidence:    []string{safeDialSummary(err)},
		}
	}
	conn.Close()
	return core.ProbeResult{
		Name:    "TCP " + addr,
		Status:  core.StatusPass,
		Latency: latency,
	}
}

// ProbeTLS establishes a TCP connection and then performs a TLS handshake.
func (p *NetworkProber) ProbeTLS(ctx context.Context, host string, port int) core.ProbeResult {
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
		return core.ProbeResult{
			Name:        "TLS " + addr,
			Status:      core.StatusFail,
			FailureKind: classifyDialError(err),
			Latency:     time.Since(start),
			Evidence:    []string{safeDialSummary(err)},
		}
	}

	tlsConn := tls.Client(rawConn, tlsCfg)
	if err := tlsConn.HandshakeContext(dialCtx); err != nil {
		rawConn.Close()
		return core.ProbeResult{
			Name:        "TLS " + addr,
			Status:      core.StatusFail,
			FailureKind: classifyTLSError(err),
			Latency:     time.Since(start),
			Evidence:    []string{safeTLSSummary(err)},
		}
	}

	state := tlsConn.ConnectionState()
	tlsConn.Close()
	return core.ProbeResult{
		Name:    "TLS " + addr,
		Status:  core.StatusPass,
		Latency: time.Since(start),
		Evidence: []string{
			"version: " + tlsVersionString(state.Version),
		},
	}
}

// ProbeTarget orchestrates the DNS -> TCP -> TLS chain.
func (p *NetworkProber) ProbeTarget(ctx context.Context, baseURL string) []core.ProbeResult {
	scheme, host, port, err := ParseTarget(baseURL)
	if err != nil {
		return []core.ProbeResult{{
			Name:        "Parse " + baseURL,
			Status:      core.StatusFail,
			FailureKind: "invalid_url",
			Evidence:    []string{err.Error()},
		}}
	}

	var results []core.ProbeResult

	// 1. DNS
	dnsRes := p.ProbeDNS(ctx, host)
	results = append(results, dnsRes)
	if dnsRes.Status != core.StatusPass {
		results = append(results, core.ProbeResult{
			Name:     "TCP " + net.JoinHostPort(host, strconv.Itoa(port)),
			Status:   core.StatusSkip,
			Evidence: []string{"skipped due to DNS failure"},
		})
		if scheme == "https" {
			results = append(results, core.ProbeResult{
				Name:     "TLS " + net.JoinHostPort(host, strconv.Itoa(port)),
				Status:   core.StatusSkip,
				Evidence: []string{"skipped due to DNS failure"},
			})
		}
		return results
	}

	// 2. TCP
	tcpRes := p.ProbeTCP(ctx, host, port)
	results = append(results, tcpRes)
	if tcpRes.Status != core.StatusPass {
		if scheme == "https" {
			results = append(results, core.ProbeResult{
				Name:     "TLS " + net.JoinHostPort(host, strconv.Itoa(port)),
				Status:   core.StatusSkip,
				Evidence: []string{"skipped due to TCP failure"},
			})
		}
		return results
	}

	// 3. TLS
	if scheme == "https" {
		tlsRes := p.ProbeTLS(ctx, host, port)
		results = append(results, tlsRes)
	}

	return results
}

// -- Error Classification Helpers --

func classifyDNSError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "dns_timeout"
	}
	return "dns_error"
}

func classifyDialError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "tcp_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "tcp_timeout"
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

func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "connection refused") || // Linux, macOS
		strings.Contains(s, "actively refused") || // Windows
		strings.Contains(s, "No connection could be made") // Windows
}

func classifyTLSError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "tls_timeout"
	}
	s := err.Error()
	if strings.Contains(s, "certificate") || strings.Contains(s, "x509") {
		return "tls_cert_error"
	}
	return "tls_error"
}

func safeDNSSummary(err error) string {
	if classifyDNSError(err) == "dns_timeout" {
		return "DNS lookup timed out"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return "hostname not found"
		}
	}
	return "DNS lookup failed"
}

func safeDialSummary(err error) string {
	switch classifyDialError(err) {
	case "tcp_refused":
		return "connection actively refused by target"
	case "tcp_timeout":
		return "connection attempt timed out"
	case "dns_error":
		return safeDNSSummary(err)
	default:
		return "TCP connection failed"
	}
}

func safeTLSSummary(err error) string {
	switch classifyTLSError(err) {
	case "tls_timeout":
		return "TLS handshake timed out"
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

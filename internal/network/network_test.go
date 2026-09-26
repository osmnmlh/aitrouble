package network

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/osmnmlh/aitrouble/internal/core"
)

// -- DNS Tests --

func TestProbeDNS_Success(t *testing.T) {
	p := &NetworkProber{
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return []string{"192.168.1.1"}, nil
		},
		Timeout: time.Second,
	}

	res := p.ProbeDNS(context.Background(), "example.com")
	if res.Status != core.StatusPass {
		t.Errorf("expected pass, got %v", res.Status)
	}
	if len(res.Evidence) == 0 || res.Evidence[0] != "resolved 1 address(es)" {
		t.Errorf("unexpected evidence: %v", res.Evidence)
	}
}

func TestProbeDNS_NotFound(t *testing.T) {
	p := &NetworkProber{
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return nil, &net.DNSError{IsNotFound: true}
		},
		Timeout: time.Second,
	}

	res := p.ProbeDNS(context.Background(), "invalid.local")
	if res.Status != core.StatusFail || res.FailureKind != "dns_error" {
		t.Errorf("expected fail/dns_error, got %v/%v", res.Status, res.FailureKind)
	}
	if res.Evidence[0] != "hostname not found" {
		t.Errorf("expected hostname not found, got %v", res.Evidence[0])
	}
}

func TestProbeDNS_Timeout(t *testing.T) {
	p := &NetworkProber{
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return nil, context.DeadlineExceeded
		},
		Timeout: time.Second,
	}

	res := p.ProbeDNS(context.Background(), "slow.local")
	if res.Status != core.StatusFail || res.FailureKind != "dns_timeout" {
		t.Errorf("expected fail/dns_timeout, got %v/%v", res.Status, res.FailureKind)
	}
}

func TestProbeDNS_Cancellation(t *testing.T) {
	p := &NetworkProber{
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return nil, context.Canceled
		},
		Timeout: time.Second,
	}

	res := p.ProbeDNS(context.Background(), "slow.local")
	if res.Status != core.StatusFail || res.FailureKind != "dns_timeout" {
		t.Errorf("expected fail/dns_timeout (for context cancel), got %v/%v", res.Status, res.FailureKind)
	}
}

// -- TCP Tests --

// fakeConn implements net.Conn minimally
type fakeConn struct{ net.Conn }

func (fakeConn) Close() error { return nil }

func TestProbeTCP_Success(t *testing.T) {
	p := &NetworkProber{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return fakeConn{}, nil
		},
		Timeout: time.Second,
	}

	res := p.ProbeTCP(context.Background(), "example.com", 80)
	if res.Status != core.StatusPass {
		t.Errorf("expected pass, got %v", res.Status)
	}
}

func TestProbeTCP_ConnectionRefused(t *testing.T) {
	p := &NetworkProber{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, errors.New("dial tcp 127.0.0.1:80: connect: connection refused")
		},
		Timeout: time.Second,
	}

	res := p.ProbeTCP(context.Background(), "example.com", 80)
	if res.Status != core.StatusFail || res.FailureKind != "tcp_refused" {
		t.Errorf("expected fail/tcp_refused, got %v/%v", res.Status, res.FailureKind)
	}
	if res.Evidence[0] != "connection actively refused by target" {
		t.Errorf("unexpected evidence: %v", res.Evidence[0])
	}
}

func TestProbeTCP_Timeout(t *testing.T) {
	p := &NetworkProber{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, context.DeadlineExceeded
		},
		Timeout: time.Second,
	}

	res := p.ProbeTCP(context.Background(), "example.com", 80)
	if res.Status != core.StatusFail || res.FailureKind != "tcp_timeout" {
		t.Errorf("expected fail/tcp_timeout, got %v/%v", res.Status, res.FailureKind)
	}
}

func TestProbeTCP_Cancellation(t *testing.T) {
	p := &NetworkProber{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, context.Canceled
		},
		Timeout: time.Second,
	}

	res := p.ProbeTCP(context.Background(), "example.com", 80)
	if res.Status != core.StatusFail || res.FailureKind != "tcp_timeout" {
		t.Errorf("expected fail/tcp_timeout (cancelled), got %v/%v", res.Status, res.FailureKind)
	}
}

// -- TLS Tests --

func TestProbeTLS_Success(t *testing.T) {
	ts := httptest.NewTLSServer(nil)
	defer ts.Close()

	host, portStr, _ := net.SplitHostPort(ts.Listener.Addr().String())
	port, _ := net.LookupPort("tcp", portStr)

	// Since we are dialing a local httptest server, we must accept its cert.
	p := &NetworkProber{
		Dial:      (&net.Dialer{}).DialContext,
		TLSConfig: &tls.Config{InsecureSkipVerify: true}, // only for test!
		Timeout:   time.Second,
	}

	res := p.ProbeTLS(context.Background(), host, port)
	if res.Status != core.StatusPass {
		t.Errorf("expected pass, got %v: %v", res.Status, res.Evidence)
	}
}

func TestProbeTLS_CertificateFailure(t *testing.T) {
	ts := httptest.NewTLSServer(nil)
	defer ts.Close()

	host, portStr, _ := net.SplitHostPort(ts.Listener.Addr().String())
	port, _ := net.LookupPort("tcp", portStr)

	// Do NOT skip verify. The cert is for "example.com" or self-signed, will fail.
	p := &NetworkProber{
		Dial:    (&net.Dialer{}).DialContext,
		Timeout: time.Second,
		// No TLSConfig overrides, defaults to secure.
	}

	res := p.ProbeTLS(context.Background(), host, port)
	if res.Status != core.StatusFail || res.FailureKind != "tls_cert_error" {
		t.Errorf("expected fail/tls_cert_error, got %v/%v", res.Status, res.FailureKind)
	}
}

func TestProbeTLS_Timeout(t *testing.T) {
	// We inject a fake Dial that returns a connection which stalls
	// Not easily mockable in stdlib without a real pipe, so we'll just mock Dial
	// to return context.DeadlineExceeded.
	p := &NetworkProber{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, context.DeadlineExceeded
		},
		Timeout: time.Second,
	}

	// This fails at Dial phase, but since it's ProbeTLS, it returns tcp_timeout via Dial error
	res := p.ProbeTLS(context.Background(), "example.com", 443)
	if res.Status != core.StatusFail || res.FailureKind != "tcp_timeout" {
		t.Errorf("expected fail/tcp_timeout, got %v/%v", res.Status, res.FailureKind)
	}
}

// -- ParseTarget Tests --

func TestParseTarget(t *testing.T) {
	tests := []struct {
		url       string
		expScheme string
		expHost   string
		expPort   int
		expectErr bool
	}{
		{"https://example.com/v1", "https", "example.com", 443, false},
		{"https://example.com:8443/v1", "https", "example.com", 8443, false},
		{"http://localhost/v1", "http", "localhost", 80, false},
		{"http://localhost:8080/v1?q=1#frag", "http", "localhost", 8080, false},
		{"ftp://example.com", "", "", 0, true},
		{"not-a-url", "", "", 0, true},
	}

	for _, tt := range tests {
		s, h, p, err := ParseTarget(tt.url)
		if tt.expectErr {
			if err == nil {
				t.Errorf("expected error for %s, got nil", tt.url)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for %s: %v", tt.url, err)
			}
			if s != tt.expScheme || h != tt.expHost || p != tt.expPort {
				t.Errorf("for %s, expected %s/%s/%d, got %s/%s/%d", tt.url, tt.expScheme, tt.expHost, tt.expPort, s, h, p)
			}
		}
	}
}

// -- Short circuit orchestration tests --

func TestProbeTarget_HTTP(t *testing.T) {
	p := &NetworkProber{
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return []string{"127.0.0.1"}, nil
		},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return fakeConn{}, nil
		},
		Timeout: time.Second,
	}

	results := p.ProbeTarget(context.Background(), "http://example.com/v1")
	if len(results) != 2 {
		t.Fatalf("expected 2 results (DNS, TCP) for HTTP, got %d", len(results))
	}
	if results[0].Name != "DNS example.com" || results[0].Status != core.StatusPass {
		t.Errorf("DNS failed: %v", results[0])
	}
	if !strings.HasPrefix(results[1].Name, "TCP") || results[1].Status != core.StatusPass {
		t.Errorf("TCP failed: %v", results[1])
	}
}

func TestProbeTarget_HTTPS_DNSFail(t *testing.T) {
	p := &NetworkProber{
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return nil, &net.DNSError{IsNotFound: true}
		},
		Timeout: time.Second,
	}

	results := p.ProbeTarget(context.Background(), "https://example.com/v1")
	if len(results) != 3 {
		t.Fatalf("expected 3 results (DNS fail, TCP skip, TLS skip), got %d", len(results))
	}
	if results[0].Status != core.StatusFail {
		t.Errorf("DNS should fail, got %v", results[0].Status)
	}
	if results[1].Status != core.StatusSkip || results[2].Status != core.StatusSkip {
		t.Errorf("TCP and TLS should skip")
	}
}

func TestProbeTarget_HTTPS_TCPFail(t *testing.T) {
	p := &NetworkProber{
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return []string{"127.0.0.1"}, nil
		},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, errors.New("connection refused")
		},
		Timeout: time.Second,
	}

	results := p.ProbeTarget(context.Background(), "https://example.com/v1")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Status != core.StatusPass {
		t.Errorf("DNS should pass")
	}
	if results[1].Status != core.StatusFail {
		t.Errorf("TCP should fail")
	}
	if results[2].Status != core.StatusSkip {
		t.Errorf("TLS should skip")
	}
}

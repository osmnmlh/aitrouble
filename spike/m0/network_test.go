package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeDialer returns a DialContextFunc that always returns the given error.
func fakeDialer(err error) DialContextFunc {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		return nil, err
	}
}

// fakeDNSLookup returns a DNSLookupFunc that returns the given values.
func fakeDNSLookup(addrs []string, err error) DNSLookupFunc {
	return func(_ context.Context, _ string) ([]string, error) {
		return addrs, err
	}
}

// blockedDialer blocks until ctx is done, then returns ctx.Err().
func blockedDialer() DialContextFunc {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

// ── DNS probe tests ───────────────────────────────────────────────────────────

func TestProbeDNS_Success(t *testing.T) {
	p := &NetworkProber{
		DNSLookup: fakeDNSLookup([]string{"93.184.216.34"}, nil),
		Timeout:   5 * time.Second,
	}
	r := p.ProbeDNS(context.Background(), "example.com")
	assertStatus(t, r, StatusPass)
	if r.FailureKind != "" {
		t.Errorf("pass should have empty FailureKind, got %q", r.FailureKind)
	}
}

func TestProbeDNS_Error(t *testing.T) {
	dnsErr := &net.DNSError{
		Name:       "notexist.invalid",
		Err:        "no such host",
		IsNotFound: true,
	}
	p := &NetworkProber{
		DNSLookup: fakeDNSLookup(nil, dnsErr),
		Timeout:   5 * time.Second,
	}
	r := p.ProbeDNS(context.Background(), "notexist.invalid")
	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "dns_error")
}

// ── TCP probe tests ───────────────────────────────────────────────────────────

// Success: use a real local listener — most deterministic approach for success.
func TestProbeTCP_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Accept and close in background to handle the probe's connection.
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	p := DefaultNetworkProber(5 * time.Second)
	r := p.ProbeTCP(context.Background(), "127.0.0.1", port)

	assertStatus(t, r, StatusPass)
	if r.FailureKind != "" {
		t.Errorf("pass should have empty FailureKind, got %q", r.FailureKind)
	}
}

// Refused: fake dialer returns a connection-refused-like error.
// Using a fake avoids OS-specific behaviour and makes the test deterministic
// across all platforms without depending on actual port state.
func TestProbeTCP_Refused(t *testing.T) {
	p := &NetworkProber{
		Dial:    fakeDialer(fmt.Errorf("dial tcp: connect: connection refused")),
		Timeout: 5 * time.Second,
	}
	r := p.ProbeTCP(context.Background(), "127.0.0.1", 1)
	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "tcp_refused")
}

// Timeout: fake dialer blocks until context deadline, then returns DeadlineExceeded.
func TestProbeTCP_Timeout(t *testing.T) {
	p := &NetworkProber{
		Dial:    blockedDialer(),
		Timeout: 50 * time.Millisecond, // short deadline for fast test
	}
	r := p.ProbeTCP(context.Background(), "192.0.2.1", 443) // TEST-NET-1 (RFC 5737)
	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "tcp_timeout")
}

// DNS error: fake dialer returns a *net.DNSError.
func TestProbeTCP_DNSError(t *testing.T) {
	p := &NetworkProber{
		Dial: fakeDialer(&net.OpError{
			Op:  "dial",
			Err: &net.DNSError{Name: "notexist.invalid", Err: "no such host", IsNotFound: true},
		}),
		Timeout: 5 * time.Second,
	}
	r := p.ProbeTCP(context.Background(), "notexist.invalid", 443)
	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "dns_error")
}

// Cancellation: context is cancelled before dial returns.
func TestProbeTCP_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	p := &NetworkProber{
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return nil, fmt.Errorf("should not reach here")
			}
		},
		Timeout: 5 * time.Second,
	}
	r := p.ProbeTCP(ctx, "localhost", 443)
	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "tcp_cancelled")
}

// ── TLS probe tests ───────────────────────────────────────────────────────────

// Success: use httptest.NewTLSServer; inject InsecureSkipVerify for test cert.
func TestProbeTLS_Success(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// The test server's client has a custom CA that trusts the test cert.
	// Extract the TLS config from it so our prober trusts the same cert.
	testTLSConfig := ts.Client().Transport.(*http.Transport).TLSClientConfig.Clone()

	host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(ts.URL, "https://"))
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	p := DefaultNetworkProber(5 * time.Second)
	p.TLSConfig = testTLSConfig

	r := p.ProbeTLS(context.Background(), host, port)
	assertStatus(t, r, StatusPass)
}

// TLS failure: real TLS server with self-signed cert, but default prober (doesn't trust it).
func TestProbeTLS_CertError(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(ts.URL, "https://"))
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	// Default prober uses real net.Dialer and default TLS config (verifies certs).
	// Because ts uses a self-signed cert not in the system root, this will fail.
	p := DefaultNetworkProber(5 * time.Second)

	r := p.ProbeTLS(context.Background(), host, port)
	assertStatus(t, r, StatusFail)
	assertFailureKind(t, r, "tls_cert_error")
}

// ── Error classification unit tests ──────────────────────────────────────────

func TestClassifyDialError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"deadline", context.DeadlineExceeded, "tcp_timeout"},
		{"cancelled", context.Canceled, "tcp_cancelled"},
		{"dns error", &net.DNSError{IsNotFound: true}, "dns_error"},
		{"refused linux", fmt.Errorf("connect: connection refused"), "tcp_refused"},
		{"refused windows", fmt.Errorf("No connection could be made"), "tcp_refused"},
		{"other", fmt.Errorf("some other error"), "tcp_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyDialError(tc.err)
			if got != tc.want {
				t.Errorf("classifyDialError(%v): want %q, got %q", tc.err, tc.want, got)
			}
		})
	}
}

// ── Test helpers ──────────────────────────────────────────────────────────────

func assertStatus(t *testing.T, r ProbeResult, want CheckStatus) {
	t.Helper()
	if r.Status != want {
		t.Errorf("Status: want %q, got %q (evidence: %v)", want, r.Status, r.Evidence)
	}
}

func assertFailureKind(t *testing.T, r ProbeResult, want string) {
	t.Helper()
	if r.FailureKind != want {
		t.Errorf("FailureKind: want %q, got %q", want, r.FailureKind)
	}
}

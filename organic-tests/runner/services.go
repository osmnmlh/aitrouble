package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"
)

// controlledProvider is real local TCP/HTTP infrastructure.  It deliberately
// knows nothing about aitrouble's implementation; it records only the request
// that arrived at the HTTP server.
type controlledProvider struct {
	mode     string
	delay    time.Duration
	server   *httptest.Server
	mu       sync.Mutex
	requests []ProviderRequest
}

func newControlledProvider(mode string, delay time.Duration) *controlledProvider {
	p := &controlledProvider{mode: mode, delay: delay}
	p.server = httptest.NewServer(http.HandlerFunc(p.serveHTTP))
	return p
}

func (p *controlledProvider) serveHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.requests = append(p.requests, ProviderRequest{Method: r.Method, Path: r.URL.RequestURI(), AuthorizationPresent: r.Header.Get("Authorization") != "", At: time.Now().UTC()})
	p.mu.Unlock()

	if p.mode == "slow" {
		time.Sleep(p.delay)
		return
	}
	if p.mode == "connection_close" {
		if h, ok := w.(http.Hijacker); ok {
			c, _, _ := h.Hijack()
			_ = c.Close()
			return
		}
		return
	}
	status := 200
	body := `{"data":[]}`
	switch p.mode {
	case "invalid_json":
		body = `{not-json`
	case "missing_data":
		body = `{"object":"list"}`
	case "401":
		status, body = 401, `{"error":{"code":"invalid_api_key","type":"authentication_error"}}`
	case "403":
		status, body = 403, `{"error":{"code":"forbidden","type":"authentication_error"}}`
	case "404":
		status, body = 404, `{"error":{"message":"missing"}}`
	case "429":
		status, body = 429, `{"error":{"message":"slow down"}}`
	case "400":
		status, body = 400, `{"error":{"message":"bad request"}}`
	case "500", "502", "503":
		fmt.Sscanf(p.mode, "%d", &status)
		body = `{"error":{"message":"service error"}}`
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (p *controlledProvider) baseURL(path string) string {
	return strings.TrimRight(p.server.URL, "/") + path
}
func (p *controlledProvider) close()         { p.server.Close() }
func (p *controlledProvider) resetRequests() { p.mu.Lock(); defer p.mu.Unlock(); p.requests = nil }
func (p *controlledProvider) observation() Observation {
	p.mu.Lock()
	defer p.mu.Unlock()
	o := Observation{ProviderRequests: len(p.requests)}
	for _, r := range p.requests {
		o.ProviderMethods = append(o.ProviderMethods, r.Method)
		o.ProviderPaths = append(o.ProviderPaths, r.Path)
		o.AuthorizationPresent = append(o.AuthorizationPresent, r.AuthorizationPresent)
	}
	return o
}

// tlsFailureServer accepts TCP so that the product's TCP probe succeeds, then
// either serves an untrusted TLS certificate or closes the TLS handshake.
type tlsFailureServer struct {
	listener    net.Listener
	httpServer  *httptest.Server
	url         string
	mode        string
	mu          sync.Mutex
	connections int
	done        chan struct{}
}

func newPlainHandshakeFailureServer() (*tlsFailureServer, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &tlsFailureServer{listener: l, url: "https://" + l.Addr().String() + "/v1", mode: "handshake_close", done: make(chan struct{})}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				close(s.done)
				return
			}
			s.mu.Lock()
			s.connections++
			s.mu.Unlock()
			_ = c.Close()
		}
	}()
	return s, nil
}

func newUntrustedTLSServer() *tlsFailureServer {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	return &tlsFailureServer{listener: ts.Listener, httpServer: ts, url: ts.URL + "/v1", mode: "untrusted_cert", done: make(chan struct{})}
}

func (s *tlsFailureServer) close() error {
	if s.httpServer != nil {
		s.httpServer.Close()
		return nil
	}
	err := s.listener.Close()
	<-s.done
	return err
}

func (s *tlsFailureServer) address() string { return s.listener.Addr().String() }

// processMarkerConfig is deliberately harmless if accidentally executed. It
// writes only to the scenario-owned processes directory.
func processMarkerConfig(marker string) string {
	return fmt.Sprintf(`{"mcpServers":{"sentinel":{"command":"cmd.exe","args":["/c","echo organic-mcp-executed > %s"],"env":{"TOKEN":"unused"}}}}`, strings.ReplaceAll(marker, `\`, `\\`))
}

func waitForFile(path string, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(15 * time.Millisecond)
	}
	return false
}

package provider

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
)

func TestClassifyHTTPError_D3(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"EOF", io.EOF, "http_error"},
		{"UnexpectedEOF", io.ErrUnexpectedEOF, "http_error"},
		{"ECONNRESET", syscall.ECONNRESET, "http_error"},
		{"WSAECONNABORTED", syscall.Errno(10053), "http_error"},
		{"WSAECONNRESET", syscall.Errno(10054), "http_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyHTTPError(tt.err)
			if got != tt.want {
				t.Errorf("classifyHTTPError(%T) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// tls error
type fakeTLSError struct{}
func (fakeTLSError) Error() string { return "tls: bad certificate" }

// To test TLS handshake close mapping to TLS layer, we'd need to mock it in network.go, but here we just test provider classifier.

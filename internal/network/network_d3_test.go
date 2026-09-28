package network

import (
	"context"
	"io"
	"syscall"
	"testing"
)

func TestIsConnectionClosed(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"EOF", io.EOF, true},
		{"UnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"ECONNRESET", syscall.ECONNRESET, true},
		{"WSAECONNABORTED", syscall.Errno(10053), true},
		{"WSAECONNRESET", syscall.Errno(10054), true},
		{"Canceled", context.Canceled, false},
		{"Nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConnectionClosed(tt.err); got != tt.want {
				t.Errorf("IsConnectionClosed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassifyTLSError_EOF(t *testing.T) {
	// A TLS handshake close (EOF) must map to the TLS layer (tls_error)
	got := classifyTLSError(io.EOF)
	if got != "tls_error" {
		t.Errorf("classifyTLSError(io.EOF) = %v, want tls_error", got)
	}
}

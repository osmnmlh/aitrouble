package core

import (
	"bytes"
	"testing"
)

func TestParseDotEnv_Variants(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected map[string]string
	}{
		{
			name:  "KEY=\"value\" # comment",
			input: []byte("KEY=\"value\" # comment\n"),
			expected: map[string]string{"KEY": "value"},
		},
		{
			name:  "KEY='value' # comment",
			input: []byte("KEY='value' # comment\n"),
			expected: map[string]string{"KEY": "value"},
		},
		{
			name:  "KEY=value # comment",
			input: []byte("KEY=value # comment\n"),
			expected: map[string]string{"KEY": "value"},
		},
		{
			name:  "KEY=http://h/p#frag",
			input: []byte("KEY=http://h/p#frag\n"),
			expected: map[string]string{"KEY": "http://h/p#frag"},
		},
		{
			name:  "KEY=\"a # b\"",
			input: []byte("KEY=\"a # b\"\n"),
			expected: map[string]string{"KEY": "a # b"},
		},
		{
			name:  "KEY=\"\"",
			input: []byte("KEY=\"\"\n"),
			expected: map[string]string{"KEY": ""},
		},
		{
			name:  "KEY=",
			input: []byte("KEY=\n"),
			expected: map[string]string{"KEY": ""},
		},
		{
			name:  "export KEY=value with whitespace",
			input: []byte("  export KEY=value  \n"),
			expected: map[string]string{"KEY": "value"},
		},
		{
			name:  "BOM CRLF",
			input: []byte("\xEF\xBB\xBFKEY=value\r\n"),
			expected: map[string]string{"KEY": "value"},
		},
		{
			name:  "missing trailing newline",
			input: []byte("KEY=value"),
			expected: map[string]string{"KEY": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseDotEnv(bytes.NewReader(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(res) != len(tt.expected) {
				t.Errorf("expected len %d, got %d", len(tt.expected), len(res))
			}
			for k, v := range tt.expected {
				if res[k] != v {
					t.Errorf("key %q: expected %q, got %q", k, v, res[k])
				}
			}
		})
	}
}

package frontier

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "normal URL",
			input:    "https://example.com/docs",
			expected: "https://example.com/docs",
		},
		{
			name:     "lowercases scheme and host",
			input:    "HTTPS://EXAMPLE.COM/docs",
			expected: "https://example.com/docs",
		},
		{
			name:     "removes fragment",
			input:    "https://example.com/docs#section",
			expected: "https://example.com/docs",
		},
		{
			name:     "removes default HTTPS port",
			input:    "https://example.com:443/docs",
			expected: "https://example.com/docs",
		},
		{
			name:     "sorts query parameters",
			input:    "https://example.com/search?z=2&a=1",
			expected: "https://example.com/search?a=1&z=2",
		},
		{
			name:    "rejects FTP",
			input:   "ftp://example.com/file",
			wantErr: true,
		},
		{
			name:    "rejects missing host",
			input:   "https:///docs",
			wantErr: true,
		},
		{
			name:    "rejects user information",
			input:   "https://user:password@example.com/",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := NormalizeURL(test.input)

			if test.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got URL %q", actual)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if actual != test.expected {
				t.Errorf(
					"expected %q, got %q",
					test.expected,
					actual,
				)
			}
		})
	}
}
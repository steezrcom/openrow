package net

import "testing"

func TestValidateOutboundURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"public https", "https://example.com", false},
		{"http rejected by default", "http://example.com", true},
		{"loopback", "https://127.0.0.1/", true},
		{"loopback v6", "https://[::1]/", true},
		{"link local", "https://169.254.169.254/", true},
		{"private 10.x", "https://10.0.0.5/", true},
		{"private 192.168", "https://192.168.1.10/", true},
		{"private 172.16", "https://172.16.0.1/", true},
		{"empty", "", true},
		{"bad scheme", "ftp://x.example.com/", true},
		{"missing host", "https:///path", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOutboundURL(tc.url, false)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	t.Run("private allowed when allowInternal=true", func(t *testing.T) {
		if err := ValidateOutboundURL("https://10.0.0.5/", true); err != nil {
			t.Fatalf("expected nil with allowInternal=true, got %v", err)
		}
	})
}

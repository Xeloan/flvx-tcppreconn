package socket

import "testing"

func TestParseListenAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantHost string
		wantPort string
	}{
		{
			name:     "defaults empty host to ipv4 wildcard",
			addr:     ":32000",
			wantHost: "0.0.0.0",
			wantPort: "32000",
		},
		{
			name:     "keeps ipv4 wildcard",
			addr:     "0.0.0.0:32001",
			wantHost: "0.0.0.0",
			wantPort: "32001",
		},
		{
			name:     "keeps ipv6 wildcard",
			addr:     "[::]:32002",
			wantHost: "::",
			wantPort: "32002",
		},
		{
			name:     "keeps specific ipv6 address",
			addr:     "[2001:db8::1]:32003",
			wantHost: "2001:db8::1",
			wantPort: "32003",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort, err := parseListenAddr(tt.addr)
			if err != nil {
				t.Fatalf("parseListenAddr returned error: %v", err)
			}
			if gotHost != tt.wantHost {
				t.Fatalf("expected host %q, got %q", tt.wantHost, gotHost)
			}
			if gotPort != tt.wantPort {
				t.Fatalf("expected port %q, got %q", tt.wantPort, gotPort)
			}
		})
	}
}

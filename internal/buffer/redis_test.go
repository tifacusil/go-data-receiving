package buffer

import "testing"

func TestParseConnString(t *testing.T) {
	tests := []struct {
		in       string
		wantAddr string
		wantPass string
	}{
		{"localhost:6379", "localhost:6379", ""},
		{"204.168.203.32:6379,password=secret", "204.168.203.32:6379", "secret"},
		{"redis:6379,password=p,ssl=false,abortConnect=false", "redis:6379", "p"},
	}
	for _, tc := range tests {
		opt, err := parseConnString(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if opt.Addr != tc.wantAddr {
			t.Errorf("%q addr = %q, want %q", tc.in, opt.Addr, tc.wantAddr)
		}
		if opt.Password != tc.wantPass {
			t.Errorf("%q pass = %q, want %q", tc.in, opt.Password, tc.wantPass)
		}
	}
}

func TestParseConnString_NoHost(t *testing.T) {
	if _, err := parseConnString("password=only"); err == nil {
		t.Fatal("expected error when no host:port present")
	}
}

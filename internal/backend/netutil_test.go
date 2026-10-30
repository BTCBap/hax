package backend

import "testing"

func TestValidateHostRejectsURLEscapes(t *testing.T) {
	bad := []string{
		"127.0.0.1@attacker.example",
		"attacker.example/path",
		"attacker.example?query=1",
		"attacker.example#fragment",
		"attacker.example:8332",
		"host name",
		"",
	}
	for _, host := range bad {
		if err := ValidateHost(host); err == nil {
			t.Errorf("ValidateHost(%q) accepted an invalid host", host)
		}
	}
}

func TestValidateHostAcceptsBareHostAndIPv6(t *testing.T) {
	for _, host := range []string{"localhost", "electrum.example.com", "127.0.0.1", "::1", "[::1]"} {
		if err := ValidateHost(host); err != nil {
			t.Errorf("ValidateHost(%q) = %v", host, err)
		}
	}
}

func TestRPCBaseURLDoesNotRedirectCredentials(t *testing.T) {
	got, err := RPCBaseURL("http", "127.0.0.1", 8332)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:8332/" {
		t.Fatalf("URL = %q", got)
	}
	if _, err := RPCBaseURL("http", "127.0.0.1@attacker.example", 8332); err == nil {
		t.Fatal("expected userinfo host to be rejected")
	}
}

func TestHostIsPrivate(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.10.20.30", true},
		{"localhost", true},
		{"LOCALHOST", true},
		{"localhost.", true},
		{"app.localhost", true},
		{"::1", true},
		{"[::1]", true},
		{"10.0.0.1", true},
		{"10.255.255.254", true},
		{"172.16.0.1", true},
		{"172.31.255.254", true},
		{"192.168.1.10", true},
		{"100.64.0.1", true},
		{"100.127.255.254", true},
		{"169.254.10.20", true},
		{"fc00::1", true},
		{"fd00::1", true},
		{"[fd00::1]", true},
		{"fe80::1", true},
		{"umbrel.local", true},
		{"UMBREL.LOCAL", true},
		{"umbrel.local.", true},
		{"printer.lan", true},
		{"node.home.arpa", true},
		{"nas.internal", true},
		{"umbrel", true},
		{"raspberrypi", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"172.15.0.1", false},
		{"172.32.0.1", false},
		{"192.169.0.1", false},
		{"100.63.255.255", false},
		{"100.128.0.0", false},
		{"2001:4860:4860::8888", false},
		{"fec0::1", false},
		{"electrum.blockstream.info", false},
		{"example.com", false},
		{"abc.onion", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := HostIsPrivate(tt.host); got != tt.want {
			t.Errorf("HostIsPrivate(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestHostTrustHelpers(t *testing.T) {
	if !HostIsLoopback("127.0.0.1") || !HostIsLoopback("localhost") || HostIsLoopback("example.com") {
		t.Error("loopback classification is wrong")
	}
	if !HostIsOnion("abc.onion") || HostIsOnion("notonion.example") {
		t.Error("onion classification is wrong")
	}
}

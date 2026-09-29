package netsafe_test

import (
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/netsafe"
)

func TestValidateURL(t *testing.T) {
	if _, err := netsafe.ValidateURL("https://example.com/path"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"file:///etc/passwd",
		"http://127.0.0.1/",
		"http://localhost/",
		"http://169.254.169.254/latest",
		"http://[::1]/",
		"ftp://example.com/",
		"",
	} {
		if _, err := netsafe.ValidateURL(raw); err == nil {
			t.Fatalf("expected block for %q", raw)
		}
	}
}

func TestValidateResolved(t *testing.T) {
	if err := netsafe.ValidateResolved("example.com", []net.IP{net.ParseIP("93.184.216.34")}); err != nil {
		t.Fatal(err)
	}
	err := netsafe.ValidateResolved("evil", []net.IP{net.ParseIP("10.0.0.1")})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err = %v", err)
	}
}

func TestIsBlockedIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "192.168.0.1", "169.254.169.254", "::1", "fc00::1"} {
		ip := netip.MustParseAddr(s)
		if !netsafe.IsBlockedIP(ip) {
			t.Fatalf("%s should be blocked", s)
		}
	}
	if netsafe.IsBlockedIP(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("8.8.8.8 should be allowed")
	}
}

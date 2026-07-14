package netutil

import (
	"net"
	"testing"
)

func TestLocalIPs(t *testing.T) {
	ips, err := LocalIPs()
	if err != nil {
		t.Fatalf("LocalIPs: %v", err)
	}
	// Every returned address must be a valid, global-unicast IP (no loopback,
	// link-local, or unspecified), and the list must be de-duplicated.
	seen := map[string]bool{}
	for _, s := range ips {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Errorf("not a valid IP: %q", s)
			continue
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			t.Errorf("non-assignable IP leaked: %q", s)
		}
		if !ip.IsGlobalUnicast() {
			t.Errorf("not global unicast: %q", s)
		}
		if seen[s] {
			t.Errorf("duplicate IP: %q", s)
		}
		seen[s] = true
	}
	// IPv4 addresses must sort before IPv6.
	sawV6 := false
	for _, s := range ips {
		is4 := net.ParseIP(s).To4() != nil
		if is4 && sawV6 {
			t.Errorf("IPv4 %q sorted after an IPv6 address", s)
		}
		if !is4 {
			sawV6 = true
		}
	}
}

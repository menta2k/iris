package biz

import (
	"strings"
	"testing"
)

// TestRelayHostsAuthoritative pins the 3.0.0 semantics: the listener relay
// allowlist is authoritative with no implicit default. An empty list denies
// relay to everyone (inbound-only / MX listener); a populated list renders
// exactly those hosts.
func TestRelayHostsAuthoritative(t *testing.T) {
	mk := func(name, ip string, relay []string) *Listener {
		return &Listener{
			ID: name, Name: name, IPAddress: ip, Port: 25,
			Hostname: "mx.example.com", RelayHosts: relay, Status: ListenerStatusActive,
		}
	}

	snap := ConfigSnapshot{
		Listeners: []*Listener{
			mk("inbound", "203.0.113.10", nil),                                   // empty -> loopback only
			mk("submission", "203.0.113.11", []string{"10.1.111.0/24"}),          // loopback + allowlist
			mk("dedup", "203.0.113.12", []string{"127.0.0.1/32", "10.2.0.0/16"}), // explicit loopback must not duplicate
		},
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}

	// Empty list renders loopback-only (localhost always relays; nothing else) —
	// NOT the old RFC-1918 default.
	if !strings.Contains(r.Content, `relay_hosts = { "127.0.0.1/32" },`) {
		t.Fatalf("inbound listener should render loopback-only relay_hosts:\n%s", r.Content)
	}
	if strings.Contains(r.Content, "10.0.0.0/8") || strings.Contains(r.Content, "192.168.0.0/16") {
		t.Fatalf("no implicit RFC-1918 relay default must be rendered:\n%s", r.Content)
	}
	// Populated list renders loopback PLUS the configured allowlist.
	if !strings.Contains(r.Content, `relay_hosts = { "127.0.0.1/32", "10.1.111.0/24" },`) {
		t.Fatalf("submission listener should render loopback + its allowlist:\n%s", r.Content)
	}
	// Operator explicitly listing loopback must not duplicate it.
	if !strings.Contains(r.Content, `relay_hosts = { "127.0.0.1/32", "10.2.0.0/16" },`) {
		t.Fatalf("explicit loopback must dedup (no duplicate entry):\n%s", r.Content)
	}
}

package biz

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// extractLuaLocal pulls the value of `local NAME = "value"` from rendered policy.
func extractLuaLocal(t *testing.T, content, name string) string {
	t.Helper()
	re := regexp.MustCompile(`local ` + regexp.QuoteMeta(name) + `\s*=\s*"([^"]*)"`)
	m := re.FindStringSubmatch(content)
	if m == nil {
		t.Fatalf("local %s not found in rendered policy", name)
	}
	return m[1]
}

func codes(s string) []rune { return []rune(s) }

// TestDMARCRepro_HiddenCharBreaksCatcher reproduces the kmx symptom:
// a non-printing character in the LOCAL PART of the configured DMARC report
// address survives rendering into DMARC_REPORT_ADDR (used by the reception-hook
// EXACT-address catcher) while DMARC_REPORT_DOMAIN is derived clean (used by
// get_listener_domain). Result at runtime: RCPT is relayed (domain matches, 250)
// but the catcher never fires (address mismatch) -> the report is received but
// never routed to DMARC_TRACKER -> never captured.
func TestDMARCRepro_HiddenCharBreaksCatcher(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "l1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	}

	const zwsp = "​" // zero-width space (typical copy-paste contaminant)
	// What kumod actually sees as the recipient for inbound report mail:
	const actualRcpt = "dmarc@kmx.example.com"

	cases := []struct {
		name string
		addr string
	}{
		{"clean", "dmarc@kmx.example.com"},
		{"hidden-char-localpart", "dmarc" + zwsp + "@kmx.example.com"},
		{"trailing-nbsp", "dmarc@kmx.example.com "},
		{"trailing-zwsp", "dmarc@kmx.example.com" + zwsp},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := base
			snap.DMARCReportAddr = tc.addr
			snap.LogStreamRedisURL = "redis://redis:6379"
			r, err := RenderKumoConfig(snap)
			if err != nil || !r.Valid {
				t.Fatalf("render: err=%v valid=%v", err, r.Valid)
			}
			gotAddr := extractLuaLocal(t, r.Content, "DMARC_REPORT_ADDR")
			gotDomain := extractLuaLocal(t, r.Content, "DMARC_REPORT_DOMAIN")

			// Simulate the two runtime decisions kumod makes against the CLEAN
			// recipient kumod observes:
			listenerRelays := gotDomain != "" && gotDomain == RecipientDomain(actualRcpt) // get_listener_domain DMARC branch
			catcherFires := strings.ToLower(actualRcpt) == gotAddr                        // reception-hook exact match

			t.Logf("addr=%q codes=%v", gotAddr, codes(gotAddr))
			t.Logf("domain=%q codes=%v", gotDomain, codes(gotDomain))
			t.Logf("listenerRelays=%v  catcherFires=%v", listenerRelays, catcherFires)

			// After SanitizeAddress, EVERY variant (clean or contaminated) must
			// render to the clean address+domain so kumod both relays the RCPT and
			// fires the catcher -> the report is captured. Before the fix, the
			// hidden-char-localpart case relayed but never fired (received but not
			// captured) — exactly the kmx symptom.
			if gotAddr != actualRcpt {
				t.Fatalf("DMARC_REPORT_ADDR not sanitized: %q codes=%v", gotAddr, codes(gotAddr))
			}
			if gotDomain != RecipientDomain(actualRcpt) {
				t.Fatalf("DMARC_REPORT_DOMAIN not sanitized: %q codes=%v", gotDomain, codes(gotDomain))
			}
			if !(listenerRelays && catcherFires) {
				t.Fatalf("%s: report not captured: relays=%v fires=%v", tc.name, listenerRelays, catcherFires)
			}
			fmt.Printf("[OK %s] sanitized -> relays=%v fires=%v -> captured\n", tc.name, listenerRelays, catcherFires)
		})
	}
}

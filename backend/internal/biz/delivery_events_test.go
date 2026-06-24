package biz

import "testing"

func TestShouldSuppressOnHardBounce(t *testing.T) {
	cases := []struct {
		classification string
		want           bool
	}{
		// Empty classification (classifier off / no match) preserves the prior
		// suppress-on-hard-bounce behavior.
		{"", true},
		// Genuine recipient-level failures suppress.
		{"InvalidRecipient", true},
		{"InactiveMailbox", true},
		// Remote policy/content — not the recipient's fault.
		{"SpamBlock", false},
		{"QuotaIssue", false},
		{"RelayDenied", false},
		// Routing / infrastructure on iris's side — must never suppress (this is
		// the loopback/prohibited-MX case that wrongly suppressed server-lab.info).
		{"RoutingErrors", false},
		{"BadConnection", false},
		{"DNSFailure", false},
		{"NoAnswerFromHost", false},
		{"BadConfiguration", false},
	}
	for _, tc := range cases {
		b := &BounceRecord{SMTPStatus: "550", Classification: tc.classification}
		if got := b.ShouldSuppressOnHardBounce(); got != tc.want {
			t.Errorf("ShouldSuppressOnHardBounce(%q) = %v, want %v", tc.classification, got, tc.want)
		}
	}
}

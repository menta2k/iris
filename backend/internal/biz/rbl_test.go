package biz

import (
	"testing"
)

func rblResultFor(rep *RBLReport, ip string) *RBLIPResult {
	for i := range rep.Results {
		if rep.Results[i].IP == ip {
			return &rep.Results[i]
		}
	}
	return nil
}

func TestRBLCheck(t *testing.T) {
	snap := ConfigSnapshot{
		Listeners: []*Listener{{IPAddress: "1.2.3.4"}, {IPAddress: "2001:db8::1"}}, // IPv6 skipped
		VMTAs:     []*VMTA{{IPAddress: "5.6.7.8", Status: VMTAStatusActive}},
	}
	// 1.2.3.4 is listed on zen.spamhaus.org; 5.6.7.8 is clean everywhere.
	dns := fakeResolver{
		host: map[string][]string{
			"4.3.2.1.zen.spamhaus.org": {"127.0.0.2"},
		},
		txt: map[string][]string{
			"4.3.2.1.zen.spamhaus.org": {"https://www.spamhaus.org/query/ip/1.2.3.4"},
		},
	}
	rep, err := NewRBLUsecase(fakeLoader{snap: snap}, dns).Check(ownerCheckCtx())
	if err != nil {
		t.Fatalf("rbl check: %v", err)
	}

	listed := rblResultFor(rep, "1.2.3.4")
	if listed == nil || !listed.Listed || listed.Source != "listener" {
		t.Fatalf("1.2.3.4 should be listed (listener): %+v", listed)
	}
	var zenReason string
	for _, l := range listed.Listings {
		if l.Zone == "zen.spamhaus.org" {
			if !l.Listed {
				t.Fatal("zen listing should be Listed")
			}
			zenReason = l.Reason
		}
	}
	if zenReason == "" {
		t.Fatal("expected a reason (TXT) for the zen listing")
	}

	clean := rblResultFor(rep, "5.6.7.8")
	if clean == nil || clean.Listed || clean.Source != "egress" {
		t.Fatalf("5.6.7.8 should be clean (egress): %+v", clean)
	}

	// The IPv6 listener is reported as skipped, not checked.
	if len(rep.Skipped) != 1 || rep.Skipped[0] != "2001:db8::1" {
		t.Fatalf("expected IPv6 skipped, got %+v", rep.Skipped)
	}
}

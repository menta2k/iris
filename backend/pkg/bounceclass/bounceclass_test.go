package bounceclass

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		status string
		want   Category
	}{
		// Hard bounces — by-detail
		{"5.1.1", CategoryUnknownUser},
		{"5.1.6", CategoryUnknownUser},
		{"5.1.2", CategoryRoutingFailed},
		{"5.1.10", CategoryRoutingFailed},
		{"5.2.1", CategoryMailboxDisabled},
		{"5.2.2", CategoryMailboxFull},
		{"5.2.3", CategoryContentRejected},
		{"5.7.1", CategoryReputationBlock},
		{"5.7.0", CategoryPolicyBlock},
		{"5.7.26", CategoryAuthFailed},
		{"5.6.1", CategoryContentRejected},
		// Hard bounces — by-subject fallback
		{"5.4.99", CategoryRoutingFailed},
		{"5.1.99", CategoryUnknownUser},
		// Hard, fallthrough
		{"5.9.99", CategoryHardOther},
		// Transient
		{"4.2.2", CategoryMailboxFull},
		{"4.7.1", CategoryTransientSpam},
		{"4.4.7", CategoryTransientNet},
		{"4.0.0", CategoryTransientOther},
		// Bad input
		{"", CategoryUnknown},
		{"5", CategoryUnknown},
		{"5.1", CategoryUnknown},
		{"junk", CategoryUnknown},
	}
	for _, c := range cases {
		got := Classify(c.status)
		if got != c.want {
			t.Errorf("Classify(%q) = %q want %q", c.status, got, c.want)
		}
	}
}

func TestIsHardIsTransient(t *testing.T) {
	if !IsHard("5.1.1") {
		t.Error("expected 5.1.1 to be hard")
	}
	if IsTransient("5.1.1") {
		t.Error("expected 5.1.1 not transient")
	}
	if !IsTransient("4.2.2") {
		t.Error("expected 4.2.2 transient")
	}
	if IsHard("4.2.2") {
		t.Error("expected 4.2.2 not hard")
	}
	// Missing / unparseable: neither.
	for _, s := range []string{"", "junk", "5"} {
		if IsHard(s) || IsTransient(s) {
			t.Errorf("missing/malformed status %q should be neither hard nor transient", s)
		}
	}
}

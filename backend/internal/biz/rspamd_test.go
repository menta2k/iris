package biz

import "testing"

func TestRspamdIsSpam(t *testing.T) {
	spam := []string{RspamdReject, RspamdRewriteSubj, RspamdAddHeader}
	for _, a := range spam {
		if !(&RspamdFilterResult{Action: a}).IsSpam() {
			t.Fatalf("action %q should be spam", a)
		}
	}
	ham := []string{RspamdNoAction, RspamdGreylist}
	for _, a := range ham {
		if (&RspamdFilterResult{Action: a}).IsSpam() {
			t.Fatalf("action %q should not be spam", a)
		}
	}
}

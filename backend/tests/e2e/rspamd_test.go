//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// TestRspamdEnforce proves the generated rspamd integration works in a real
// kumod: inbound mail to a hosted domain is scanned via the policy's
// iris_rspamd_scan (an HTTP call to /checkv2), and in enforce mode a "reject"
// verdict blocks the message at reception while a clean message is delivered,
// tagged with the rspamd headers the policy prepends.
//
// The fake rspamd returns "reject" for recipients containing "spam".
func TestRspamdEnforce(t *testing.T) {
	requireE2E(t)
	requireDocker(t)

	rspamdURL := startRspamdStub(t)

	snap := routingSnapshot(kumodIP)
	snap.RspamdMode = "enforce"
	snap.RspamdURL = rspamdURL
	// sink.test is a hosted (inbound) domain, so its mail is scanned. A
	// recipient-domain route delivers accepted mail to a VMTA (and thus the sink).
	snap.HostedDomains = []string{"sink.test"}
	snap.Routes = append(snap.Routes, &biz.RoutingRule{
		ID: "r-hosted", Name: "hosted", MatchType: biz.MatchRecipientDomain, MatchValue: "sink.test",
		Priority: 50, TargetType: biz.TargetVMTA, TargetID: "v-bulk", Status: biz.RoutingStatusActive,
	})
	r := startRig(t, snap)

	// Spam: rspamd "reject" → enforce → kumod rejects at reception (5xx).
	out, err := r.tryInject("", "spam@sink.test", "")
	if err == nil {
		t.Fatalf("expected rspamd enforce to reject spam, but injection succeeded:\n%s", out)
	}
	if !strings.Contains(out, "550") {
		t.Fatalf("expected a 5xx rejection for spam, got:\n%s", out)
	}

	// Ham: rspamd "no action" → accepted, scanned, delivered with rspamd headers.
	r.inject("ham@sink.test")
	msgs := r.waitForSink(1, 30*time.Second)
	if !strings.Contains(msgs[0].Data, "X-Spam-Score") || !strings.Contains(msgs[0].Data, "X-Rspamd-Action") {
		t.Fatalf("delivered ham missing rspamd scan headers:\n%s", headerBlock(msgs[0].Data))
	}

	// And the rejected spam never reached the sink.
	for _, m := range r.sinkMessages() {
		for _, rc := range m.Rcpts {
			if strings.Contains(rc, "spam@") {
				t.Fatalf("rejected spam must not be delivered, but sink saw %s", rc)
			}
		}
	}
}

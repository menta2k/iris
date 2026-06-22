//go:build e2e

// Package e2e contains live KumoMTA end-to-end tests: they render an iris
// config, load it into a real kumod, and assert kumod actually behaves as the
// config intends (routing, logging, DKIM, webhooks, DSN/bounce, feedback).
//
// The suite is opt-in. It is excluded from the default build by the `e2e` build
// tag and skips at runtime unless IRIS_E2E=1, so `go test ./...` never touches
// it. Run it with `make test-e2e` (which brings up the compose `e2e` profile).
//
// See specs/001-kumomta-admin-ui/e2e-test-plan.md for the full design.
package e2e

import (
	"os"
	"os/exec"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
)

// kumoImage is the KumoMTA container image the harness loads policies into. It
// defaults to the latest official image and is overridable so CI / local runs
// can pin a digest.
func kumoImage() string {
	if v := os.Getenv("IRIS_E2E_KUMO_IMAGE"); v != "" {
		return v
	}
	return "ghcr.io/kumocorp/kumomta:latest"
}

// requireE2E skips the calling test unless the suite is explicitly enabled.
// Mirrors the IRIS_TEST_DSN gating used by the DB integration suite.
func requireE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("IRIS_E2E") != "1" {
		t.Skip("IRIS_E2E not set to 1; skipping live KumoMTA e2e test")
	}
}

// requireDocker skips the calling test when the docker CLI is unavailable. The
// live suite drives a real kumod container, so docker is a hard prerequisite.
func requireDocker(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker not found on PATH; skipping live KumoMTA e2e test")
	}
	return path
}

// representativeSnapshot builds a ConfigSnapshot that exercises every feature
// the renderer emits, so loading it into kumod validates the full surface in
// one shot: multiple listeners, active/draining/disabled VMTAs, a weighted
// group, mailclass + recipient routing, ready/unready DKIM signers,
// suppressions, inbound rspamd enforcement, the Redis log hook, the bounce/DSN
// pipeline, and the delivery-rate retry schedule.
//
// Domains and addresses are RFC 2606 / RFC 5737 reserved (example.com,
// 203.0.113.0/24) so nothing here can touch real infrastructure.
// harnessDKIMKey returns a real PEM key for the representative snapshot: the
// renderer re-validates DKIM key material, and signing keys are now stored
// inline (PEM) rather than referenced by path.
func harnessDKIMKey() string {
	pem, err := biz.GenerateDKIMPrivateKey()
	if err != nil {
		panic(err)
	}
	return pem
}

func representativeSnapshot() biz.ConfigSnapshot {
	return biz.ConfigSnapshot{
		Listeners: []*biz.Listener{
			{ID: "lst-1", Name: "edge", IPAddress: "198.51.100.1", Port: 2525, Hostname: "mx.example.com",
				MaxMessageSize: 26214400, Status: biz.ListenerStatusActive},
			{ID: "lst-2", Name: "submission", IPAddress: "198.51.100.2", Port: 2587, Hostname: "submit.example.com",
				RelayHosts: []string{"10.0.0.0/8"}, Status: biz.ListenerStatusActive},
			{ID: "lst-off", Name: "legacy", IPAddress: "198.51.100.3", Port: 2526, Hostname: "old.example.com",
				Status: biz.ListenerStatusDisabled},
		},
		VMTAs: []*biz.VMTA{
			{ID: "v1", Name: "vmta-a", ListenerID: "lst-1", IPAddress: "203.0.113.10", EHLOName: "a.example.com",
				MaxConnections: 10, Status: biz.VMTAStatusActive},
			{ID: "v2", Name: "vmta-b", ListenerID: "lst-1", IPAddress: "203.0.113.11", EHLOName: "b.example.com",
				Status: biz.VMTAStatusActive},
			{ID: "v3", Name: "vmta-drain", ListenerID: "lst-1", IPAddress: "203.0.113.12", EHLOName: "c.example.com",
				Status: biz.VMTAStatusDraining},
			{ID: "v4", Name: "vmta-off", ListenerID: "lst-1", IPAddress: "203.0.113.13", EHLOName: "d.example.com",
				Status: biz.VMTAStatusDisabled},
		},
		Groups: []*biz.VMTAGroup{
			{ID: "g1", Name: "bulk-pool", Status: biz.VMTAGroupStatusActive, Members: []biz.VMTAGroupMember{
				{VMTAID: "v1", Weight: 70}, {VMTAID: "v2", Weight: 30},
			}},
		},
		Routes: []*biz.RoutingRule{
			{ID: "r1", Name: "bulk", MatchType: biz.MatchMailclass, MatchHeader: "X-Mail-Class", MatchValue: "bulk",
				Priority: 100, TargetType: biz.TargetVMTAGroup, TargetID: "g1", Status: biz.RoutingStatusActive},
			{ID: "r2", Name: "promo", MatchType: biz.MatchMailclass, MatchHeader: "X-Campaign-Type", MatchValue: "promo",
				Priority: 90, TargetType: biz.TargetVMTA, TargetID: "v2", Status: biz.RoutingStatusActive},
			{ID: "r3", Name: "vip", MatchType: biz.MatchRecipientDomain, MatchValue: "vip.example",
				Priority: 10, TargetType: biz.TargetVMTA, TargetID: "v1", Status: biz.RoutingStatusActive},
			{ID: "r4", Name: "lab-subnet", MatchType: biz.MatchSenderIP, MatchValue: "10.1.111.0/24",
				AssignMailclass: "bulk", Priority: 200, Status: biz.RoutingStatusActive},
			{ID: "r5", Name: "test-host", MatchType: biz.MatchSenderIP, MatchValue: "192.0.2.50",
				AssignMailclass: "bulk", Priority: 150, Status: biz.RoutingStatusActive},
		},
		InboundWebhooks: []*biz.WebhookRule{
			{ID: "wh1", Name: "support", MatchType: biz.MatchRecipientEmail, MatchValue: "support@hooked.example",
				DestinationURL: "https://hooks.example/iris", SecretRef: "webhook-secret", Status: biz.WebhookActive},
			{ID: "wh2", Name: "leads", MatchType: biz.MatchRecipientDomain, MatchValue: "leads.example",
				DestinationURL: "https://hooks.example/leads", Status: biz.WebhookActive},
		},
		DKIM: []*biz.DKIMDomain{
			{ID: "d1", Domain: "example.com", Selector: "s1", PrivateKeyRef: harnessDKIMKey(), Status: biz.DKIMReady},
			{ID: "d2", Domain: "pending.com", Selector: "s1", PrivateKeyRef: harnessDKIMKey(), Status: biz.DKIMNeedsAttention},
		},
		Suppressions: []*biz.SuppressionEntry{
			{ID: "s1", Type: biz.SuppressEmail, Value: "blocked@example.com", Status: biz.SuppressActive},
			{ID: "s2", Type: biz.SuppressDomain, Value: "blocked.example", Status: biz.SuppressActive},
		},
		EgressEHLODefault:      "mail.example.com",
		RspamdMode:             "enforce",
		RspamdURL:              "http://rspamd.example:11334",
		LogStreamRedisURL:      "redis://redis:6379",
		LogStreamName:          "iris.mail.events",
		EsmtpListen:            "0.0.0.0:2525",
		HTTPListen:             "0.0.0.0:8000",
		EgressRetryInterval:    "20m",
		EgressMaxRetryInterval: "2h",
		EgressMaxAge:           "1d",
		BounceDomain:           "bounce.example.com",
		GeneratorVersion:       "iris-e2e/0.1.0",
	}
}

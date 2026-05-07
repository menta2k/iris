package kumopolicy

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func goodSnapshot() *Snapshot {
	return &Snapshot{
		GlobalSettings: GlobalSettings{
			LogDir:        "/var/log/kumomta",
			SpoolDir:      "/var/spool/kumomta",
			PolicyVersion: "v1",
		},
		Listeners: []Listener{{
			Name: "mx-public", ListenAddr: "0.0.0.0:25", Hostname: "mx.example.com",
			TLSEnabled: true, TLSCertPath: "/etc/ssl/cert.pem", TLSKeyPath: "/etc/ssl/key.pem",
			Domains: []ListenerDomain{{Domain: "example.com", RelayAllowed: true}},
		}},
		DkimIdentities: []DkimIdentity{{
			Domain: "example.com", Selector: "k1", Algorithm: "ed25519",
			KeyPath: "/etc/kumo/dkim/example.com_k1.key",
		}},
		VirtualMtas: []VirtualMta{{
			Name: "egress-1", SourceIPs: []string{"203.0.113.4"},
			HeloName: "mx.example.com", MaxConnections: 50, MaxMessagesPerConnection: 100,
			ConnectTimeout: 30, ProviderProfile: "default",
		}},
		MailClasses: []MailClass{{
			Name: "transactional", Enabled: true,
			TargetKind: "vmta", TargetRef: "egress-1",
		}},
		RoutingRules: []RoutingRule{{
			Name: "send-via-egress-1", Priority: 50, Enabled: true,
			Conditions: []RuleCondition{{Field: "to_domain", Op: "endswith", Value: "example.com"}},
			Target:     RuleTarget{Kind: "vmta", Ref: "egress-1"},
		}},
		Suppressions: []Suppression{{Address: "blocked@example.com", Scope: "address"}},
	}
}

func TestRenderProducesValidLua(t *testing.T) {
	snap := goodSnapshot()
	out, err := Render(snap, RenderOptions{
		GeneratedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		GeneratedBy: "alice",
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Lua)
	require.Len(t, out.SHA256, 64)

	// Smoke checks for content presence.
	require.Contains(t, out.Lua, "kumo.start_esmtp_listener")
	require.Contains(t, out.Lua, `"egress-1"`)
	require.Contains(t, out.Lua, `dkim_sign`)
	// Suppressions are no longer table-embedded — the renderer emits a
	// Redis-backed memoized lookup. Whether the live (memoized) check or
	// the no-Redis stub is emitted depends on goodSnapshot's
	// LogStreamRedisURL; either way the route_message body must call it.
	require.Contains(t, out.Lua, "is_suppressed(rcpt, rdom)")
	// And critically: NO suppressed address should appear inline. This is
	// the property the old SUPPRESSED_ADDR table required us to defend
	// against; the new lookup achieves it for free.
	require.NotContains(t, out.Lua, "blocked@example.com")

	require.Empty(t, Lint(out.Lua))
}

func TestRenderIsDeterministic(t *testing.T) {
	snap := goodSnapshot()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a, err := Render(snap, RenderOptions{GeneratedAt: at})
	require.NoError(t, err)
	b, err := Render(snap, RenderOptions{GeneratedAt: at})
	require.NoError(t, err)
	require.Equal(t, a.SHA256, b.SHA256)
}

func TestRenderRefusesInvalidSnapshot(t *testing.T) {
	bad := goodSnapshot()
	bad.Listeners[0].Hostname = "not a host;rm -rf /"
	_, err := Render(bad, RenderOptions{})
	require.Error(t, err)
	var v *ValidationError
	require.ErrorAs(t, err, &v)
}

// TestRenderDoesNotEmbedSuppressions confirms that no suppression value
// reaches the rendered Lua at all — the new design pushes those into
// Redis at the service layer, so a hostile address cannot Lua-inject
// because there's no Lua emit path to inject through.
func TestRenderDoesNotEmbedSuppressions(t *testing.T) {
	snap := goodSnapshot()
	snap.Suppressions = append(snap.Suppressions, Suppression{
		Address: `pwn@example.com"; os.execute("evil"); --`,
		Scope:   "address",
	})
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	require.Empty(t, Lint(out.Lua))
	require.NotContains(t, out.Lua, "pwn@example.com")
	require.NotContains(t, out.Lua, "os.execute")
}

func TestRenderInjectionViaRoutingValue(t *testing.T) {
	snap := goodSnapshot()
	snap.RoutingRules = append(snap.RoutingRules, RoutingRule{
		Name: "x", Enabled: true,
		Conditions: []RuleCondition{{
			Field: "header.subject", Op: "contains",
			Value: `"; require('os').exit(); --`,
		}},
		Target: RuleTarget{Kind: "discard"},
	})
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	// Lint proves the source is syntactically valid Lua. This is the
	// load-bearing safety property: a Lua-injection that succeeded in
	// escaping the literal would either trigger a parse error or
	// re-arrange the statement structure.
	require.Empty(t, Lint(out.Lua))
	// Defense-in-depth: the input's leading `"` must appear only as the
	// escaped form `\"`. Locate the payload body and check the two bytes
	// immediately preceding it.
	idx := strings.Index(out.Lua, `; require('os').exit()`)
	require.Greater(t, idx, 1, "payload must be present in rendered Lua")
	require.Equal(t, `\"`, out.Lua[idx-2:idx],
		"the input's leading quote must have been emitted as the escape sequence \\\"")
}

func TestValidateRejectsBadConditionField(t *testing.T) {
	snap := goodSnapshot()
	snap.RoutingRules[0].Conditions[0].Field = "header.x-evil"
	require.Error(t, snap.Validate())
}

func TestValidateRejectsBadRegex(t *testing.T) {
	snap := goodSnapshot()
	snap.RoutingRules[0].Conditions[0].Op = "regex"
	snap.RoutingRules[0].Conditions[0].Value = `(unclosed`
	require.Error(t, snap.Validate())
}

func TestValidateRejectsRejectCode(t *testing.T) {
	snap := goodSnapshot()
	snap.RoutingRules[0].Target = RuleTarget{Kind: "reject", RejectCode: 200, RejectText: "oops"}
	require.Error(t, snap.Validate())
}

func TestValidateRejectsNonIPSourceIPs(t *testing.T) {
	snap := goodSnapshot()
	snap.VirtualMtas[0].SourceIPs = []string{"not-an-ip"}
	require.Error(t, snap.Validate())
}

func TestValidateRejectsBadDkimAlgo(t *testing.T) {
	snap := goodSnapshot()
	snap.DkimIdentities[0].Algorithm = "rsa-512"
	require.Error(t, snap.Validate())
}

func TestSanitizeCommentStripsNewlines(t *testing.T) {
	require.Equal(t, "a b c", sanitizeComment("a\nb\rc"))
	require.NotContains(t, sanitizeComment("\nfoo\n--injected"), "\n")
}

func TestRenderEmptySnapshotWorks(t *testing.T) {
	empty := &Snapshot{GlobalSettings: GlobalSettings{LogDir: "/var/log/kumo", SpoolDir: "/var/spool/kumo"}}
	out, err := Render(empty, RenderOptions{})
	require.NoError(t, err)
	require.True(t, strings.Contains(out.Lua, "kumo.on('init'"))
	require.Empty(t, Lint(out.Lua))
}

// TestRenderBounceSingleDomain pins the legacy single-domain mode shape:
// every outbound funnels through one bounce domain via the fallback const,
// the catcher accepts only that one domain.
func TestRenderBounceSingleDomain(t *testing.T) {
	snap := &Snapshot{GlobalSettings: GlobalSettings{
		LogStreamRedisURL: "redis://r:6379/0",
		VerpSecret:        "supersecret",
		BounceDomain:      "bounces.example.com",
	}}
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	require.Contains(t, out.Lua, `local BOUNCE_DOMAIN_FALLBACK  = "bounces.example.com"`)
	require.Contains(t, out.Lua, `["bounces.example.com"] = true,`)
	// Multi-domain map must be empty in legacy mode — the fallback handles
	// every sender, no per-sender override needed.
	require.Contains(t, out.Lua, "local BOUNCE_SENDER_TO_BOUNCE = {\n}")
	require.Contains(t, out.Lua, "kumo.on('make.dsn_xadd'")
	require.Contains(t, out.Lua, "msg:set_sender(string.format('b+%s.%s@%s'")
}

// TestRenderBounceMultiDomain pins the multi-domain shape: per-sender
// bounce subdomains derived from the prefix convention, the catcher
// accepts every derived bounce domain, the fallback constant is empty so
// senders not in the configured list go un-rewritten.
func TestRenderBounceMultiDomain(t *testing.T) {
	snap := &Snapshot{GlobalSettings: GlobalSettings{
		LogStreamRedisURL:   "redis://r:6379/0",
		VerpSecret:          "supersecret",
		BounceSenderDomains: []string{"test-1.com", "Test2.COM", "test-1.com"}, // dup + casing
	}}
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)

	// Sender → bounce map: lowercased + de-duped.
	require.Contains(t, out.Lua, `["test-1.com"] = "bounces.test-1.com",`)
	require.Contains(t, out.Lua, `["test2.com"] = "bounces.test2.com",`)
	require.NotContains(t, out.Lua, `Test-1.com`) // proves casing is normalised
	require.NotContains(t, out.Lua, `Test2.COM`)

	// Catcher accepts both derived bounce subdomains.
	require.Contains(t, out.Lua, `["bounces.test-1.com"] = true,`)
	require.Contains(t, out.Lua, `["bounces.test2.com"] = true,`)

	// Fallback constant must be empty in multi mode so out-of-list
	// senders aren't accidentally rewritten to an unaligned domain.
	require.Contains(t, out.Lua, `local BOUNCE_DOMAIN_FALLBACK  = ""`)

	// Listener-domain rule emitted for both bounce subdomains.
	require.Contains(t, out.Lua, `["bounces.test-1.com"] = { relay_to = true },`)
	require.Contains(t, out.Lua, `["bounces.test2.com"] = { relay_to = true },`)
}

// TestRenderBouncePrefixOverride pins the IRIS_BOUNCE_DOMAIN_PREFIX knob.
// Operators sometimes need a non-default prefix (e.g. "rcpt." instead of
// "bounces.") to fit an existing DNS scheme.
func TestRenderBouncePrefixOverride(t *testing.T) {
	snap := &Snapshot{GlobalSettings: GlobalSettings{
		LogStreamRedisURL:   "redis://r:6379/0",
		VerpSecret:          "supersecret",
		BounceSenderDomains: []string{"test.com"},
		BouncePrefix:        "rcpt",
	}}
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	require.Contains(t, out.Lua, `["test.com"] = "rcpt.test.com",`)
	require.Contains(t, out.Lua, `["rcpt.test.com"] = true,`)
}

// TestRenderBounceMultiPrecedence: when both BounceSenderDomains AND the
// legacy BounceDomain are set, multi mode wins and the legacy fallback is
// empty (so the operator's stale BounceDomain doesn't silently catch
// sends from unmanaged domains).
func TestRenderBounceMultiPrecedence(t *testing.T) {
	snap := &Snapshot{GlobalSettings: GlobalSettings{
		LogStreamRedisURL:   "redis://r:6379/0",
		VerpSecret:          "supersecret",
		BounceDomain:        "legacy.example.com",
		BounceSenderDomains: []string{"test.com"},
	}}
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	require.Contains(t, out.Lua, `local BOUNCE_DOMAIN_FALLBACK  = ""`)
	require.NotContains(t, out.Lua, `legacy.example.com`)
	require.Contains(t, out.Lua, `["test.com"] = "bounces.test.com",`)
}

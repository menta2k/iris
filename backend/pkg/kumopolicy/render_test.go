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

// TestRenderDiagLogFilter pins the diagnostic-verbosity emit: the default
// fires when unset, an operator override is honored verbatim, and a filter
// with shell metacharacters is rejected at validation.
func TestRenderDiagLogFilter(t *testing.T) {
	def := &Snapshot{GlobalSettings: GlobalSettings{LogDir: "/var/log/kumo", SpoolDir: "/var/spool/kumo"}}
	out, err := Render(def, RenderOptions{})
	require.NoError(t, err)
	require.Contains(t, out.Lua, "kumo.set_diagnostic_log_filter(\""+DiagLogFilterDefault+"\")")

	custom := &Snapshot{GlobalSettings: GlobalSettings{
		LogDir: "/var/log/kumo", SpoolDir: "/var/spool/kumo",
		DiagLogFilter: "kumod=debug",
	}}
	out, err = Render(custom, RenderOptions{})
	require.NoError(t, err)
	require.Contains(t, out.Lua, `kumo.set_diagnostic_log_filter("kumod=debug")`)

	bad := &Snapshot{GlobalSettings: GlobalSettings{DiagLogFilter: "kumod=info; os.execute('x')"}}
	require.Error(t, bad.Validate())
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

func TestRenderAddsMissingHeaderHook(t *testing.T) {
	out, err := Render(goodSnapshot(), RenderOptions{})
	require.NoError(t, err)
	// The header-hygiene helper exists, is invoked from the routing chain,
	// and only adds Date/Message-ID when absent.
	require.Contains(t, out.Lua, "local function iris_add_missing_headers(msg)")
	require.Contains(t, out.Lua, "iris_add_missing_headers(msg)")
	require.Contains(t, out.Lua, "msg:prepend_header('Date'")
	require.Contains(t, out.Lua, "msg:prepend_header('Message-ID'")
	require.Contains(t, out.Lua, "get_first_named_header_value('Date')")
}

func TestRenderEgressEhloDefaultWhenSet(t *testing.T) {
	snap := goodSnapshot()
	snap.GlobalSettings.EgressEhloDomain = "mail.example.com"
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	// Baked as a const and applied at the egress-source level (incl. the
	// implicit 'default' source) so it can't be bypassed by an unmatched
	// routing rule.
	require.Contains(t, out.Lua, `local EGRESS_EHLO_DEFAULT = "mail.example.com"`)
	require.Contains(t, out.Lua, "if (clean.ehlo_domain == nil or clean.ehlo_domain == '') and EGRESS_EHLO_DEFAULT ~= '' then")
	// Message-ID domain is pinned to the configured EHLO FQDN.
	require.Contains(t, out.Lua, `local IRIS_MID_DOMAIN = "mail.example.com"`)
}

func TestRenderEgressEhloEmptyWhenUnset(t *testing.T) {
	out, err := Render(goodSnapshot(), RenderOptions{})
	require.NoError(t, err)
	// No default configured → empty const, so kumomta's own default
	// (system hostname) is preserved for sources without a helo_name.
	require.Contains(t, out.Lua, `local EGRESS_EHLO_DEFAULT = ""`)
}

func TestRenderQueueRetryFields(t *testing.T) {
	snap := goodSnapshot()
	snap.GlobalSettings.EgressRetryInterval = "5m"
	snap.GlobalSettings.EgressMaxRetryInterval = "2h"
	snap.GlobalSettings.EgressMaxAge = "3d"
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	require.Contains(t, out.Lua, `retry_interval = "5m",`)
	require.Contains(t, out.Lua, `max_retry_interval = "2h",`)
	require.Contains(t, out.Lua, `max_age = "3d",`)
}

func TestRenderQueueRetryOmittedWhenUnset(t *testing.T) {
	out, err := Render(goodSnapshot(), RenderOptions{})
	require.NoError(t, err)
	// The normal queue config carries only egress_pool when no retry
	// settings are configured (kumomta defaults apply).
	require.Contains(t, out.Lua, "  return kumo.make_queue_config {\n    egress_pool = pool,\n  }")
}

func TestRenderWebhookCatcher(t *testing.T) {
	snap := goodSnapshot()
	snap.MailWebhooks = []MailWebhook{
		{Name: "support", Address: "support@kmx.jobs.bg", URL: "https://hooks.example.com/in", Secret: "s3cr3t", Enabled: true},
		{Name: "catchall", Address: "inbound.example.com", URL: "https://hooks.example.com/dom", Enabled: true},
	}
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	require.Empty(t, Lint(out.Lua))
	// Lookup tables: exact email vs domain catch-all.
	require.Contains(t, out.Lua, `WEBHOOK_BY_EMAIL["support@kmx.jobs.bg"] = { url = "https://hooks.example.com/in"`)
	require.Contains(t, out.Lua, `WEBHOOK_BY_DOMAIN["inbound.example.com"] = { url = "https://hooks.example.com/dom"`)
	// Custom-lua POST queue + routing + queue-config branch + listener accept.
	require.Contains(t, out.Lua, "kumo.on('make.webhook_post'")
	require.Contains(t, out.Lua, "msg:set_meta('queue', WEBHOOK_TRACKER)")
	require.Contains(t, out.Lua, "if domain == WEBHOOK_TRACKER then")
	require.Contains(t, out.Lua, `["kmx.jobs.bg"] = { relay_to = true }`)
	require.Contains(t, out.Lua, `["inbound.example.com"] = { relay_to = true }`)
}

func TestRenderNoWebhookWhenNone(t *testing.T) {
	out, err := Render(goodSnapshot(), RenderOptions{})
	require.NoError(t, err)
	require.NotContains(t, out.Lua, "kumo.on('make.webhook_post'")
	require.NotContains(t, out.Lua, "if domain == WEBHOOK_TRACKER then")
}

func TestRenderRejectsBadWebhookURL(t *testing.T) {
	snap := goodSnapshot()
	snap.MailWebhooks = []MailWebhook{{Name: "x", Address: "a@b.com", URL: "ftp://nope", Enabled: true}}
	_, err := Render(snap, RenderOptions{})
	require.Error(t, err)
}

func TestRenderRspamdEnforce(t *testing.T) {
	snap := goodSnapshot() // goodSnapshot has a listener for example.com
	snap.GlobalSettings.RspamdMode = "enforce"
	snap.GlobalSettings.RspamdURL = "http://127.0.0.1:11333"
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	require.Empty(t, Lint(out.Lua))
	require.Contains(t, out.Lua, `local RSPAMD_URL = "http://127.0.0.1:11333"`)
	require.Contains(t, out.Lua, "local RSPAMD_ENFORCE = true")
	// rspamd is a function called from the single SMTP handler, NOT its own
	// kumo.on — KumoMTA permits only one smtp_server_message_received handler.
	require.Contains(t, out.Lua, "local function iris_rspamd_scan(msg)")
	require.Contains(t, out.Lua, "kumo.on('smtp_server_message_received', function(msg) iris_rspamd_scan(msg) route_message(msg) end)")
	require.Contains(t, out.Lua, "client:post(RSPAMD_URL .. '/checkv2')")
	// Inbound-to-hosted guard + bounce exclusion.
	require.Contains(t, out.Lua, "if not (LISTENER_DOMAINS and LISTENER_DOMAINS[rdom]) then return end")
	require.Contains(t, out.Lua, "if BOUNCE_DOMAINS and BOUNCE_DOMAINS[rdom] then return end")
	require.Contains(t, out.Lua, "kumo.reject(550")
	// Exactly ONE smtp_server_message_received registration must exist.
	require.Equal(t, 1, strings.Count(out.Lua, "kumo.on('smtp_server_message_received'"))
}

func TestRenderRspamdTagNeverRejects(t *testing.T) {
	snap := goodSnapshot()
	snap.GlobalSettings.RspamdMode = "tag"
	snap.GlobalSettings.RspamdURL = "http://127.0.0.1:11333"
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	require.Contains(t, out.Lua, "local RSPAMD_ENFORCE = false")
	require.Contains(t, out.Lua, "client:post(RSPAMD_URL .. '/checkv2')")
	require.Contains(t, out.Lua, `msg:prepend_header('X-Spam-Score'`)
}

func TestRenderRspamdOff(t *testing.T) {
	out, err := Render(goodSnapshot(), RenderOptions{})
	require.NoError(t, err)
	require.NotContains(t, out.Lua, "/checkv2")
	require.NotContains(t, out.Lua, "RSPAMD_URL")
}

func TestListenerNameAllowsDomainStyle(t *testing.T) {
	snap := goodSnapshot()
	// Operators commonly name a listener after its domain — dots allowed.
	snap.Listeners[0].Name = "server-lab.info"
	out, err := Render(snap, RenderOptions{})
	require.NoError(t, err)
	require.Empty(t, Lint(out.Lua))
}

func TestListenerNameRejectsUnsafe(t *testing.T) {
	snap := goodSnapshot()
	snap.Listeners[0].Name = "bad name;rm -rf /"
	_, err := Render(snap, RenderOptions{})
	require.Error(t, err)
}

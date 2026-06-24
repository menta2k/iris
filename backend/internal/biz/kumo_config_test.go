package biz

import (
	"strings"
	"testing"
)

// testDKIMKeyPEM is a real RSA key for render tests: the renderer re-validates
// the snapshot (including DKIM key material), so a parseable key is required.
// Live signing/verification is exercised by the e2e DKIM test.
var testDKIMKeyPEM = func() string {
	pem, err := GenerateDKIMPrivateKey()
	if err != nil {
		panic(err)
	}
	return pem
}()

func TestRenderKumoConfig(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{
			{ID: "v1", Name: "vmta-a", ListenerID: "lst-1", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive},
			{ID: "v2", Name: "vmta-b", ListenerID: "lst-1", IPAddress: "203.0.113.11", EHLOName: "b.example.com", Status: VMTAStatusActive},
			{ID: "v3", Name: "vmta-off", ListenerID: "lst-1", IPAddress: "203.0.113.12", EHLOName: "c.example.com", Status: VMTAStatusDisabled},
		},
		Groups: []*VMTAGroup{
			{ID: "g1", Name: "bulk-pool", Status: VMTAGroupStatusActive, Members: []VMTAGroupMember{
				{VMTAID: "v1", Weight: 70}, {VMTAID: "v2", Weight: 30},
			}},
		},
		Routes: []*RoutingRule{
			{ID: "r1", Name: "bulk", MatchType: MatchMailclass, MatchValue: "bulk", Priority: 100, TargetType: TargetVMTAGroup, TargetID: "g1", Status: RoutingStatusActive},
			{ID: "r2", Name: "vip", MatchType: MatchRecipientDomain, MatchValue: "vip.example", Priority: 10, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
		},
		DKIM: []*DKIMDomain{
			{ID: "d1", Domain: "example.com", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady},
			{ID: "d2", Domain: "pending.com", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMNeedsAttention},
		},
		Suppressions: []*SuppressionEntry{
			{ID: "s1", Type: SuppressEmail, Value: "blocked@example.com", Status: SuppressActive},
			{ID: "s2", Type: SuppressDomain, Value: "blocked.example", Status: SuppressActive},
			{ID: "s3", Type: SuppressEmail, Value: "old@example.com", Status: SuppressDisabled},
		},
	}

	r, err := RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// The rendered policy must be syntactically valid Lua.
	if !r.Valid {
		t.Fatalf("rendered policy failed lint: %v\n%s", r.LintIssues, r.Content)
	}

	// Only active VMTAs become egress sources.
	if r.VMTACount != 2 {
		t.Fatalf("expected 2 egress sources, got %d", r.VMTACount)
	}
	if !strings.Contains(r.Content, `SOURCES["vmta-a"] = { source_address = "203.0.113.10", ehlo_domain = "a.example.com" }`) {
		t.Fatalf("missing egress source for vmta-a:\n%s", r.Content)
	}
	if strings.Contains(r.Content, "vmta-off") {
		t.Fatal("disabled VMTA must not appear in config")
	}
	if !strings.Contains(r.Content, "kumo.make_egress_source") || !strings.Contains(r.Content, "get_queue_config") {
		t.Fatal("policy must use the real KumoMTA callback API")
	}

	// Weighted group pool.
	if r.PoolCount != 1 || !strings.Contains(r.Content, `POOLS["bulk-pool"] = { entries = { { name = "vmta-a", weight = 70 }, { name = "vmta-b", weight = 30 } } }`) {
		t.Fatalf("missing weighted pool:\n%s", r.Content)
	}

	// Routes ordered by DESCENDING priority (higher wins): bulk (100) before
	// vip (10). The mailclass rule carries its configured header name.
	if r.RouteCount != 2 {
		t.Fatalf("expected 2 routes, got %d", r.RouteCount)
	}
	bulkIdx := strings.Index(r.Content, `match_value = "bulk"`)
	vipIdx := strings.Index(r.Content, `"vip.example"`)
	if bulkIdx < 0 || vipIdx < 0 || bulkIdx > vipIdx {
		t.Fatalf("routes not ordered by descending priority:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, `match_type = "mailclass", match_header = "X-Mail-Class", match_value = "bulk"`) {
		t.Fatalf("mailclass route should carry its header name:\n%s", r.Content)
	}

	// Only ready DKIM signers. The key is emitted inline as KeySource key_data.
	if r.DKIMCount != 1 ||
		!strings.Contains(r.Content, `DKIM_BY_DOMAIN["example.com"] = { selector = "s1", key = { key_data = `) ||
		!strings.Contains(r.Content, `}, algo = "sha256" }`) {
		t.Fatalf("expected one ready DKIM signer with inline key_data:\n%s", r.Content)
	}
	if strings.Contains(r.Content, `DKIM_BY_DOMAIN["pending.com"]`) {
		t.Fatal("non-ready DKIM domain must not be a signer")
	}

	// Suppressions are no longer rendered inline — they live in Redis. The config
	// must not contain any suppression values regardless of the snapshot.
	if strings.Contains(r.Content, "SUPPRESSED_EMAILS") ||
		strings.Contains(r.Content, "blocked@example.com") ||
		strings.Contains(r.Content, "blocked.example") {
		t.Fatalf("suppression list must not be rendered into the config:\n%s", r.Content)
	}
	if r.SuppressionCount != 0 {
		t.Fatalf("SuppressionCount should be 0 (list is in Redis), got %d", r.SuppressionCount)
	}

	if r.Checksum == "" {
		t.Fatal("expected a checksum")
	}
	// Determinism.
	r2, _ := RenderKumoConfig(snap)
	if r.Checksum != r2.Checksum {
		t.Fatal("render should be deterministic")
	}
}

func TestDrainingVMTADroppedFromGroupPool(t *testing.T) {
	// A draining VMTA keeps its own source/singleton pool (so in-flight queued
	// mail can drain) but is dropped from group pool entries (so weighting stops
	// routing NEW mail to it). A disabled VMTA is excluded everywhere.
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{
			{ID: "v1", Name: "vmta-a", ListenerID: "l1", IPAddress: "203.0.113.1", EHLOName: "a.example.com", Status: VMTAStatusActive},
			{ID: "v2", Name: "vmta-drain", ListenerID: "l1", IPAddress: "203.0.113.2", EHLOName: "b.example.com", Status: VMTAStatusDraining},
			{ID: "v3", Name: "vmta-off", ListenerID: "l1", IPAddress: "203.0.113.3", EHLOName: "c.example.com", Status: VMTAStatusDisabled},
		},
		Groups: []*VMTAGroup{
			{ID: "g1", Name: "pool", Status: VMTAGroupStatusActive, Members: []VMTAGroupMember{
				{VMTAID: "v1", Weight: 50}, {VMTAID: "v2", Weight: 50}, {VMTAID: "v3", Weight: 50},
			}},
		},
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}

	// The draining VMTA still has its own source + singleton pool (drains).
	if !strings.Contains(r.Content, `SOURCES["vmta-drain"]`) ||
		!strings.Contains(r.Content, `POOLS["vmta-drain"] = { entries = { { name = "vmta-drain" } } }`) {
		t.Fatalf("draining VMTA must keep its own source/pool:\n%s", r.Content)
	}
	// The group pool contains only the active member.
	if !strings.Contains(r.Content, `POOLS["pool"] = { entries = { { name = "vmta-a", weight = 50 } } }`) {
		t.Fatalf("group pool must include only active members:\n%s", r.Content)
	}
	// Neither draining nor disabled appears as a group entry.
	if strings.Contains(r.Content, `{ name = "vmta-drain", weight`) || strings.Contains(r.Content, `{ name = "vmta-off", weight`) {
		t.Fatalf("draining/disabled VMTAs must not be weighted group entries:\n%s", r.Content)
	}
	// The disabled VMTA is absent entirely (no source).
	if strings.Contains(r.Content, "vmta-off") {
		t.Fatalf("disabled VMTA must not appear at all:\n%s", r.Content)
	}
}

func TestRenderDefinesSpoolsAndLocalLogs(t *testing.T) {
	// kumod refuses to start without a spool ("No spools have been defined").
	// Since the generated policy is the standalone --policy entrypoint (it
	// defines its own init + listeners), it must define the spools and local
	// logs itself. Regression guard for the e2e "policy fails to start" find.
	r, err := RenderKumoConfig(ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "l1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	})
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	for _, want := range []string{
		`kumo.define_spool { name = 'data', path = '/var/spool/kumomta/data' }`,
		`kumo.define_spool { name = 'meta', path = '/var/spool/kumomta/meta' }`,
		`kumo.configure_local_logs { log_dir = '/var/log/kumomta' }`,
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("generated init must contain %q:\n%s", want, r.Content)
		}
	}
}

func TestRenderDKIMSigningWiring(t *testing.T) {
	// DKIM signing must use the real KumoMTA API, verified end-to-end against a
	// live kumod: signing happens on reception (there is no
	// smtp_client_message_sending event), via kumo.dkim.rsa_sha256_signer (not
	// rsa_sha256), with the required `headers` list. Regression guard for the
	// three signing bugs the e2e DKIM test surfaced.
	r, err := RenderKumoConfig(ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "l1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		DKIM:  []*DKIMDomain{{ID: "d1", Domain: "signed.example", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady}},
	})
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	if strings.Contains(r.Content, "smtp_client_message_sending") {
		t.Fatal("DKIM must not use the non-existent smtp_client_message_sending event")
	}
	for _, want := range []string{
		"local function iris_dkim_sign(msg)",
		"kumo.dkim.rsa_sha256_signer(params)",
		"kumo.dkim.ed25519_signer(params)",
		"headers = { 'From', 'To', 'Subject', 'Date', 'Message-ID', 'MIME-Version', 'Content-Type', 'Sender' }",
		"kumo.on('http_message_generated', function(msg)",
		"iris_dkim_sign(msg)", // called from the reception hook
		// Message-ID is injected when absent, before signing, in both hooks.
		"local function iris_ensure_message_id(msg)",
		"msg:prepend_header('Message-ID', string.format('<%s@%s>', tostring(msg:id()), domain))",
		"iris_ensure_message_id(msg)",
		// Subdomain signing: a From of infra.verax.net is signed by a verax.net
		// key by walking up the parent labels; d= is the matched parent domain.
		"local function iris_dkim_lookup(from_domain)",
		"d = d:match('%.(.+)$')",
		"local sign_domain, cfg = iris_dkim_lookup(domain)",
		"domain = sign_domain,",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("DKIM signing must contain %q:\n%s", want, r.Content)
		}
	}
}

func TestRenderRspamdAndLogHook(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		DKIM:  []*DKIMDomain{{ID: "d1", Domain: "hosted.example", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady}},
	}

	// Disabled by default: rspamd is a no-op stub, log hook absent.
	off, err := RenderKumoConfig(base)
	if err != nil || !off.Valid {
		t.Fatalf("render off: err=%v valid=%v issues=%v", err, off.Valid, off.LintIssues)
	}
	if !strings.Contains(off.Content, "local function iris_rspamd_scan(_msg) end") {
		t.Fatal("expected rspamd no-op stub when disabled")
	}
	if strings.Contains(off.Content, "configure_log_hook") {
		t.Fatal("log hook must be absent when log stream is disabled")
	}

	// Enabled: rspamd enforce + log stream.
	on := base
	on.RspamdMode = "enforce"
	on.RspamdURL = "http://rspamd:11334"
	on.LogStreamRedisURL = "redis://redis:6379"
	on.LogStreamName = "iris.mail.events"
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	// rspamd is scoped to hosted domains and enforced; called at reception.
	if !strings.Contains(r.Content, `HOSTED_DOMAINS["hosted.example"] = true`) {
		t.Fatalf("hosted domain not emitted:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, "RSPAMD_ENFORCE = true") ||
		!strings.Contains(r.Content, "/checkv2") ||
		!strings.Contains(r.Content, "iris_rspamd_scan(msg)") {
		t.Fatalf("rspamd scan not wired into reception:\n%s", r.Content)
	}
	// log hook: configure_log_hook in init + XADD constructor + tracker queue.
	if !strings.Contains(r.Content, "configure_log_hook") ||
		!strings.Contains(r.Content, "make.redis_tracker") ||
		!strings.Contains(r.Content, "'XADD'") ||
		!strings.Contains(r.Content, "domain == LOGSTREAM_TRACKER") {
		t.Fatalf("log hook not fully wired:\n%s", r.Content)
	}
}

func TestRenderRequireTLSPolicy(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		TLSPolicies: []*TLSPolicy{
			{ID: "t1", Domain: "secure.example", Mode: TLSModeRequired, Status: TLSPolicyActive},
			{ID: "t2", Domain: "lab.example", Mode: TLSModeRequiredInsecure, Status: TLSPolicyActive},
			{ID: "t3", Domain: "off.example", Mode: TLSModeRequired, Status: TLSPolicyDisabled},
		},
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	for _, want := range []string{
		`REQUIRE_TLS_DOMAINS["secure.example"] = "Required"`,
		`REQUIRE_TLS_DOMAINS["lab.example"] = "RequiredInsecure"`,
		"local tls = REQUIRE_TLS_DOMAINS[string.lower(domain)]",
		"params.enable_tls = tls",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("require-TLS policy must contain %q:\n%s", want, r.Content)
		}
	}
	// A disabled policy must not be emitted.
	if strings.Contains(r.Content, "off.example") {
		t.Fatalf("disabled TLS policy must be skipped:\n%s", r.Content)
	}
	// Generous SMTP client timeouts so slow/tarpitting receivers (e.g. a stall on
	// RSET during connection reuse) don't trip KumoMTA's aggressive defaults.
	for _, want := range []string{"rset_timeout = '30s'", "idle_timeout = '60s'", "data_timeout = '60s'"} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("egress path must set %q:\n%s", want, r.Content)
		}
	}
}

func TestRenderDeliveryRatesAndBouncePipeline(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	}

	// Delivery-rate knobs flow onto the default egress queue's retry schedule.
	rates := base
	rates.EgressRetryInterval = "20m"
	rates.EgressMaxRetryInterval = "2h"
	rates.EgressMaxAge = "1d"
	r, err := RenderKumoConfig(rates)
	if err != nil || !r.Valid {
		t.Fatalf("render rates: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	if !strings.Contains(r.Content, `retry_interval = "20m"`) ||
		!strings.Contains(r.Content, `max_retry_interval = "2h"`) ||
		!strings.Contains(r.Content, `max_age = "1d"`) {
		t.Fatalf("delivery-rate retry params not emitted on the default queue:\n%s", r.Content)
	}

	// Bounce pipeline disabled unless both bounce domain AND log-stream Redis set.
	half := base
	half.BounceDomain = "bounce.example.com"
	off, err := RenderKumoConfig(half)
	if err != nil || !off.Valid {
		t.Fatalf("render bounce-half: err=%v valid=%v issues=%v", err, off.Valid, off.LintIssues)
	}
	if strings.Contains(off.Content, "make.dsn_xadd") || strings.Contains(off.Content, `DSN_TRACKER   = "iris_dsn_catcher"`) {
		t.Fatalf("bounce pipeline must stay disabled without a log-stream Redis URL:\n%s", off.Content)
	}

	// Fully enabled: DSN catcher relay + XADD constructor + bounce-domain consts.
	on := base
	on.BounceDomain = "Bounce.Example.com"
	on.LogStreamRedisURL = "redis://redis:6379"
	on.LogStreamName = "iris.mail.events"
	rb, err := RenderKumoConfig(on)
	if err != nil || !rb.Valid {
		t.Fatalf("render bounce: err=%v valid=%v issues=%v", err, rb.Valid, rb.LintIssues)
	}
	// Bounce domain is normalized to lower-case and the DSN stream is named.
	if !strings.Contains(rb.Content, `local BOUNCE_DOMAIN = "bounce.example.com"`) ||
		!strings.Contains(rb.Content, `local DSN_TRACKER   = "iris_dsn_catcher"`) ||
		!strings.Contains(rb.Content, `local DSN_STREAM    = "`+DSNStreamName+`"`) {
		t.Fatalf("bounce/DSN constants not emitted:\n%s", rb.Content)
	}
	// The reception hook routes bounce-domain mail to the DSN tracker queue, and
	// the custom_lua queue XADDs onto the DSN stream.
	if !strings.Contains(rb.Content, "make.dsn_xadd") ||
		!strings.Contains(rb.Content, "msg:set_meta('queue', DSN_TRACKER)") ||
		!strings.Contains(rb.Content, "domain == DSN_TRACKER") {
		t.Fatalf("DSN catcher not wired into reception/queue:\n%s", rb.Content)
	}
}

func TestRenderFBLListenerDomain(t *testing.T) {
	// The FBL pipeline enables log_arf for the configured domain so kumod parses
	// ARF reports and emits Feedback records. Verified end-to-end against a live
	// kumod (TestFeedbackReportAutoSuppresses).
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "l1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	}

	// Disabled: no listener-domain handler, no log_arf.
	off, err := RenderKumoConfig(base)
	if err != nil || !off.Valid {
		t.Fatalf("render off: err=%v valid=%v issues=%v", err, off.Valid, off.LintIssues)
	}
	if strings.Contains(off.Content, "log_arf") || strings.Contains(off.Content, "get_listener_domain") {
		t.Fatalf("FBL/listener-domain must be absent when unconfigured:\n%s", off.Content)
	}

	// Approved: one get_listener_domain handler with log_arf for every approved
	// FBL domain. No forward block is rendered.
	on := base
	on.FBLEndpoints = []*FBLEndpoint{
		{Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com", Status: FBLApproved},
		{Domain: "fbl2.example.com", FeedbackAddress: "fbl@fbl2.example.com", Status: FBLApproved},
	}
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	if !strings.Contains(r.Content, `FBL_DOMAINS["fbl.example.com"] = true`) ||
		!strings.Contains(r.Content, `FBL_DOMAINS["fbl2.example.com"] = true`) ||
		!strings.Contains(r.Content, "if FBL_DOMAINS[domain] then") ||
		!strings.Contains(r.Content, "log_arf = 'LogThenDrop'") {
		t.Fatalf("FBL multi-domain log_arf not wired:\n%s", r.Content)
	}
	if strings.Contains(r.Content, "FBL_FORWARD[") || strings.Contains(r.Content, "msg:set_recipient(fwd)") {
		t.Fatalf("approved-only config must not render a forward block:\n%s", r.Content)
	}
	// Exactly one get_listener_domain handler (the event may be defined once).
	if n := strings.Count(r.Content, "kumo.on('get_listener_domain'"); n != 1 {
		t.Fatalf("expected exactly 1 get_listener_domain handler, got %d", n)
	}
}

func TestRenderFBLAwaitingForward(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "l1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	}

	// An awaiting-approval endpoint relays its domain and forwards mail at its
	// feedback address to the forward address (no ARF parse).
	on := base
	on.FBLEndpoints = []*FBLEndpoint{
		{Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com", ForwardAddress: "ops@example.com", Status: FBLAwaitingApproval},
	}
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	if !strings.Contains(r.Content, `FBL_FORWARD["fbl@fbl.example.com"] = "ops@example.com"`) ||
		!strings.Contains(r.Content, `FBL_RELAY_DOMAINS["fbl.example.com"] = true`) ||
		!strings.Contains(r.Content, "if FBL_RELAY_DOMAINS[domain] then") {
		t.Fatalf("FBL awaiting-approval forward not wired:\n%s", r.Content)
	}
	// The forward re-injects a new local message (not set_recipient, which would be
	// rejected as external relaying), with a local SPF sender + egress pool, and
	// consumes the carrier via the sink queue.
	for _, want := range []string{
		"kumo.make_message('fbl-forward@' .. dom, fwd, msg:get_data())",
		"kumo.inject_message(copy)",
		"copy:set_meta('mailclass', 'fbl-forward')",
		`local FBL_FORWARD_POOL = "v1"`,
		"copy:set_meta('tenant', FBL_FORWARD_POOL)",
		"msg:set_meta('queue', FBL_FORWARD_SINK)",
		"kumo.on('make.fbl_sink'",
		"if domain == FBL_FORWARD_SINK then",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("FBL forward re-injection missing %q:\n%s", want, r.Content)
		}
	}
	// It must NOT use set_recipient on the inbound message anymore.
	if strings.Contains(r.Content, "msg:set_recipient(fwd)") {
		t.Fatalf("FBL forward must re-inject, not set_recipient on the inbound msg:\n%s", r.Content)
	}
	// Awaiting must not enable ARF parsing for the domain.
	if strings.Contains(r.Content, `FBL_DOMAINS["fbl.example.com"] = true`) {
		t.Fatalf("awaiting-approval domain must not enable log_arf:\n%s", r.Content)
	}

	// Mixed: a domain with both an approved and an awaiting endpoint → approved
	// wins (ARF for the whole domain, no forward).
	mixed := base
	mixed.FBLEndpoints = []*FBLEndpoint{
		{Domain: "fbl.example.com", FeedbackAddress: "arf@fbl.example.com", Status: FBLApproved},
		{Domain: "fbl.example.com", FeedbackAddress: "pending@fbl.example.com", ForwardAddress: "ops@example.com", Status: FBLAwaitingApproval},
	}
	rm, err := RenderKumoConfig(mixed)
	if err != nil || !rm.Valid {
		t.Fatalf("render mixed: err=%v valid=%v issues=%v", err, rm.Valid, rm.LintIssues)
	}
	if !strings.Contains(rm.Content, `FBL_DOMAINS["fbl.example.com"] = true`) {
		t.Fatalf("mixed: approved domain must still enable log_arf:\n%s", rm.Content)
	}
	if strings.Contains(rm.Content, "FBL_FORWARD[") || strings.Contains(rm.Content, `FBL_RELAY_DOMAINS["fbl.example.com"]`) {
		t.Fatalf("mixed: awaiting entry on an approved domain must be excluded from forward:\n%s", rm.Content)
	}
}

func TestMailClassification(t *testing.T) {
	// Classification (what class a mail IS) is independent of routing (where it
	// goes): a recipient rule can win the route while the mail is still tagged
	// with its class from the configured mailclass header.
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		Routes: []*RoutingRule{
			{ID: "r1", Name: "by-recipient", MatchType: MatchRecipientEmail, MatchValue: "vip@x.example",
				Priority: 200, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
			{ID: "r2", Name: "promo", MatchType: MatchMailclass, MatchHeader: "X-Campaign-Type", MatchValue: "promo",
				Priority: 100, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
			{ID: "r3", Name: "bulk", MatchType: MatchMailclass, MatchHeader: "X-Mail-Class", MatchValue: "bulk",
				Priority: 90, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
		},
		LogStreamRedisURL: "redis://redis:6379",
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	// The MAIL_CLASSES table maps header -> value -> class label.
	if !strings.Contains(r.Content, `["X-Campaign-Type"] = { ["promo"] = "promo", }`) ||
		!strings.Contains(r.Content, `["X-Mail-Class"] = { ["bulk"] = "bulk", }`) {
		t.Fatalf("MAIL_CLASSES table not emitted as expected:\n%s", r.Content)
	}
	// The reception hook classifies and sets the mailclass meta.
	if !strings.Contains(r.Content, "local function classify_mail(msg)") ||
		!strings.Contains(r.Content, "local class = classify_mail(msg)") ||
		!strings.Contains(r.Content, "msg:set_meta('mailclass', class)") {
		t.Fatalf("classification not wired into reception:\n%s", r.Content)
	}
	// The log hook serializes the mailclass meta so the consumer can read it.
	if !strings.Contains(r.Content, "meta = { 'tenant', 'mailclass' }") {
		t.Fatalf("log hook must serialize the mailclass meta:\n%s", r.Content)
	}
}

func TestSenderIPClassification(t *testing.T) {
	// A sender_ip rule assigns a mailclass to mail with no mailclass header,
	// based on the connecting client's IP/CIDR. Delivery then follows the
	// mailclass rule for that class.
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "pool-x", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		Routes: []*RoutingRule{
			{ID: "r1", Name: "test-class-route", MatchType: MatchMailclass, MatchHeader: "X-Mail-Class", MatchValue: "test-class",
				Priority: 100, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
			{ID: "r2", Name: "lab-subnet", MatchType: MatchSenderIP, MatchValue: "10.1.111.0/24",
				AssignMailclass: "test-class", Priority: 200, Status: RoutingStatusActive},
			{ID: "r3", Name: "single-ip", MatchType: MatchSenderIP, MatchValue: "10.1.111.5",
				AssignMailclass: "test-class", Priority: 150, Status: RoutingStatusActive},
		},
		LogStreamRedisURL: "redis://redis:6379",
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	// The SENDER_IP_CLASSES table carries CIDR/IP → class, highest priority first.
	if !strings.Contains(r.Content, `{ cidr = "10.1.111.0/24", mailclass = "test-class" }`) ||
		!strings.Contains(r.Content, `{ cidr = "10.1.111.5", mailclass = "test-class" }`) {
		t.Fatalf("SENDER_IP_CLASSES not emitted as expected:\n%s", r.Content)
	}
	// The classifier and CIDR matcher are wired in and consulted as a fallback.
	if !strings.Contains(r.Content, "local function classify_by_sender_ip(msg)") ||
		!strings.Contains(r.Content, "class = classify_by_sender_ip(msg)") ||
		!strings.Contains(r.Content, "local function _ip_matches(ip, spec)") {
		t.Fatalf("sender-ip classification not wired into reception:\n%s", r.Content)
	}
	// select_pool takes the resolved class so an assigned class routes to its pool.
	if !strings.Contains(r.Content, "local function select_pool(msg, recipient, class)") ||
		!strings.Contains(r.Content, "(class ~= nil and class == route.match_value)") {
		t.Fatalf("select_pool must route by the resolved class:\n%s", r.Content)
	}
	// sender_ip rules must NOT appear as ROUTES entries (they have no pool).
	if strings.Contains(r.Content, `match_type = "sender_ip"`) {
		t.Fatalf("sender_ip rules must not be emitted into ROUTES:\n%s", r.Content)
	}
}

func TestBounceVerpGeneration(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs:             []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		BounceDomain:      "bounce.example.com",
		LogStreamRedisURL: "redis://redis:6379", // bounce pipeline needs a stream
	}
	// No VERP without a secret.
	off, err := RenderKumoConfig(base)
	if err != nil || !off.Valid {
		t.Fatalf("render off: %v", err)
	}
	if strings.Contains(off.Content, "BOUNCE_VERP_SECRET") {
		t.Fatalf("VERP must be absent without a secret:\n%s", off.Content)
	}
	// With a secret, the envelope rewrite is emitted in the reception hook.
	on := base
	on.BounceVerpSecret = "verp-key"
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: %v", err)
	}
	for _, want := range []string{
		`local BOUNCE_VERP_SECRET = "verp-key"`,
		"kumo.digest.hmac_sha256({ key_data = BOUNCE_VERP_SECRET }, mid)",
		"msg:set_sender(string.format('b+%s.%s@%s', string.sub(tostring(mac), 1, 16), mid, BOUNCE_DOMAIN))",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("VERP missing %q:\n%s", want, r.Content)
		}
	}
	// The rewrite lives inside the reception hook, not a (non-firing) sending hook.
	if strings.Contains(r.Content, "smtp_client_message_sending") {
		t.Fatalf("VERP must not use smtp_client_message_sending:\n%s", r.Content)
	}
}

func TestBounceClassifierGeneration(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	}
	// Off when unset.
	off, err := RenderKumoConfig(base)
	if err != nil || !off.Valid {
		t.Fatalf("render off: %v", err)
	}
	if strings.Contains(off.Content, "configure_bounce_classifier") {
		t.Fatalf("classifier should be absent when unset:\n%s", off.Content)
	}
	// On with a file path.
	on := base
	on.BounceClassifierFile = "/opt/kumomta/share/bounce_classifier/iana.toml"
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: %v", err)
	}
	if !strings.Contains(r.Content, `kumo.configure_bounce_classifier { files = { "/opt/kumomta/share/bounce_classifier/iana.toml" } }`) {
		t.Fatalf("classifier not emitted:\n%s", r.Content)
	}
}

func TestInboundWebhookGeneration(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		InboundWebhooks: []*WebhookRule{
			{ID: "w1", Name: "support", MatchType: MatchRecipientEmail, MatchValue: "support@server-lab.info",
				DestinationURL: "https://portal.example/hook", SecretRef: "s3cr3t", Status: WebhookActive},
			{ID: "w2", Name: "dom", MatchType: MatchRecipientDomain, MatchValue: "leads.example.com",
				DestinationURL: "https://portal.example/leads", Status: WebhookActive},
		},
		LogStreamRedisURL: "redis://redis:6379",
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	// Routing tables with the secret carried through verbatim.
	if !strings.Contains(r.Content, `WEBHOOK_BY_EMAIL["support@server-lab.info"] = { url = "https://portal.example/hook", secret = "s3cr3t" }`) {
		t.Fatalf("WEBHOOK_BY_EMAIL not emitted as expected:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, `WEBHOOK_BY_DOMAIN["leads.example.com"] =`) {
		t.Fatalf("WEBHOOK_BY_DOMAIN not emitted:\n%s", r.Content)
	}
	// Relay-accept the recipient's domain (derived from the email match).
	if !strings.Contains(r.Content, `WEBHOOK_DOMAINS["server-lab.info"] = true`) ||
		!strings.Contains(r.Content, `WEBHOOK_DOMAINS["leads.example.com"] = true`) {
		t.Fatalf("WEBHOOK_DOMAINS not emitted:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, "if WEBHOOK_DOMAINS[domain] then") {
		t.Fatalf("get_listener_domain must relay webhook domains:\n%s", r.Content)
	}
	// The poster forwards the raw message exactly as the previous release did.
	for _, want := range []string{
		"kumo.on('make.webhook_post'",
		"req:header('Content-Type', 'message/rfc822')",
		"req:header('X-Iris-Recipient', email)",
		"req:header('X-Iris-Message-Id', tostring(message:id()))",
		"kumo.digest.hmac_sha256({ key_data = route.secret }, body)",
		"req:header('X-Iris-Signature', tostring(sig))",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("webhook poster missing %q:\n%s", want, r.Content)
		}
	}
	// Reception routes matched mail to the webhook queue; queue config wires it.
	if !strings.Contains(r.Content, "msg:set_meta('queue', WEBHOOK_TRACKER)") ||
		!strings.Contains(r.Content, "if domain == WEBHOOK_TRACKER then") {
		t.Fatalf("webhook reception/queue routing not wired:\n%s", r.Content)
	}
	// Webhook-captured mail is tagged with the 'webhook' class so it is
	// identifiable in the mail log (the hook returns before classify_mail).
	if !strings.Contains(r.Content, "msg:set_meta('mailclass', 'webhook')") {
		t.Fatalf("webhook mailclass tag not emitted:\n%s", r.Content)
	}
	// A recipient at a webhook-relayed domain that matches no rule is rejected so
	// the sending MTA bounces it, rather than relaying to the domain's real MX.
	if !strings.Contains(r.Content, "if WEBHOOK_DOMAINS[rdom] then") ||
		!strings.Contains(r.Content, "recipient rejected, no matching route") {
		t.Fatalf("unknown-recipient reject for webhook domains not emitted:\n%s", r.Content)
	}
}

func TestDMARCCatcherGeneration(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "l1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	}

	// Enabled (report address + redis): catcher, tracker route, listener relay.
	on := base
	on.DMARCReportAddr = "dmarc@kmx.jobs.bg"
	on.LogStreamRedisURL = "redis://redis:6379"
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	for _, want := range []string{
		`local DMARC_REPORT_ADDR   = "dmarc@kmx.jobs.bg"`,
		`local DMARC_REPORT_DOMAIN = "kmx.jobs.bg"`,
		"kumo.on('make.dmarc_xadd'",
		"if (rcpt and rcpt.email or ''):lower() == DMARC_REPORT_ADDR then",
		"msg:set_meta('queue', DMARC_TRACKER)",
		"if domain == DMARC_TRACKER then",
		"if DMARC_REPORT_DOMAIN ~= '' and domain == DMARC_REPORT_DOMAIN then",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("DMARC catcher missing %q:\n%s", want, r.Content)
		}
	}

	// Disabled: no DMARC catcher rendered.
	off, err := RenderKumoConfig(base)
	if err != nil || !off.Valid {
		t.Fatalf("render off: err=%v valid=%v issues=%v", err, off.Valid, off.LintIssues)
	}
	// The empty DMARC_* locals may still be declared, but no catcher constructor,
	// queue route, or reception match should be rendered.
	if strings.Contains(off.Content, "make.dmarc_xadd") ||
		strings.Contains(off.Content, "if domain == DMARC_TRACKER then") ||
		strings.Contains(off.Content, "== DMARC_REPORT_ADDR then") {
		t.Fatalf("DMARC catcher must be absent when unconfigured:\n%s", off.Content)
	}
}

func TestSuppressionRedisLookupGeneration(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	}

	// With Redis configured, suppression is a memoized EXISTS lookup, not a table.
	on := base
	on.LogStreamRedisURL = "redis://redis:6379"
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	for _, want := range []string{
		"kumo.memoize(_supp_lookup",
		"conn:query('EXISTS', 'supp:e:' .. recipient, 'supp:d:' .. domain)",
		"name = 'iris_suppression'",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("suppression redis lookup missing %q:\n%s", want, r.Content)
		}
	}
	if strings.Contains(r.Content, "SUPPRESSED_EMAILS") {
		t.Fatalf("must not render the inline suppression table:\n%s", r.Content)
	}

	// Without Redis, suppression enforcement degrades to a no-op stub.
	off, err := RenderKumoConfig(base)
	if err != nil || !off.Valid {
		t.Fatalf("render off: err=%v valid=%v issues=%v", err, off.Valid, off.LintIssues)
	}
	if !strings.Contains(off.Content, "local function is_suppressed(_recipient) return false end") {
		t.Fatalf("expected no-op is_suppressed when redis disabled:\n%s", off.Content)
	}
	if strings.Contains(off.Content, "kumo.memoize(_supp_lookup") {
		t.Fatalf("must not render the redis lookup when redis disabled:\n%s", off.Content)
	}
}

func TestSuppressedLogRecordGeneration(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs:             []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		Suppressions:      []*SuppressionEntry{{ID: "s1", Type: SuppressEmail, Value: "blocked@example.com", Status: SuppressActive}},
		LogStreamRedisURL: "redis://redis:6379",
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	// The reception hook emits a synthetic Suppressed record before rejecting.
	for _, want := range []string{
		"local function iris_log_suppressed(msg, recipient)",
		"type = 'Suppressed',",
		"'type', 'Suppressed', 'data', payload",
		"iris_log_suppressed(msg, recipient)",
		"kumo.reject(550, '5.7.1 recipient is suppressed')",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("suppressed logging missing %q:\n%s", want, r.Content)
		}
	}
	// The call must precede the reject so the record is streamed first.
	if i, j := strings.Index(r.Content, "iris_log_suppressed(msg, recipient)"), strings.Index(r.Content, "kumo.reject(550"); i < 0 || j < 0 || i > j {
		t.Fatalf("iris_log_suppressed must be called before kumo.reject:\n%s", r.Content)
	}
}

func TestLogHookHeadersAreDynamic(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		Routes: []*RoutingRule{
			{ID: "r1", Name: "promo", MatchType: MatchMailclass, MatchHeader: "X-Campaign-Type", MatchValue: "promo",
				Priority: 100, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
			{ID: "r2", Name: "bulk", MatchType: MatchMailclass, MatchHeader: "X-Mail-Class", MatchValue: "bulk",
				Priority: 90, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
			// A recipient rule contributes no header; a disabled mailclass rule
			// is excluded.
			{ID: "r3", Name: "rcpt", MatchType: MatchRecipientDomain, MatchValue: "x.example",
				Priority: 80, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
			{ID: "r4", Name: "off", MatchType: MatchMailclass, MatchHeader: "X-Secret", MatchValue: "z",
				Priority: 70, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusDisabled},
		},
		LogStreamRedisURL: "redis://redis:6379",
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	// From + Subject are always captured (From recovers the original sender past
	// the VERP rewrite); the configured mailclass headers follow.
	if !strings.Contains(r.Content, `headers = { "From", "Subject", "X-Campaign-Type", "X-Mail-Class" }`) {
		t.Fatalf("log hook headers not dynamic:\n%s", r.Content)
	}
	// The hardcoded X-Kumo-Mail-Class is gone, and the disabled rule's header
	// is not leaked into the allow-list.
	if strings.Contains(r.Content, "X-Kumo-Mail-Class") {
		t.Fatal("stale hardcoded X-Kumo-Mail-Class header must not appear")
	}
	if strings.Contains(r.Content, `"X-Secret"`) {
		t.Fatal("disabled mailclass rule header must not be in the log-hook allow-list")
	}
}

func TestLuaStringEscaping(t *testing.T) {
	// Quotes and backslashes are escaped; output is double-quoted.
	got, err := LuaString(`a"b\c` + "\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `"a\"b\\c\n"` {
		t.Fatalf("unexpected escaping: %s", got)
	}
	// NUL and invalid UTF-8 are rejected.
	if _, err := LuaString("a\x00b"); err == nil {
		t.Fatal("NUL must be rejected")
	}
	if _, err := LuaString("\xff\xfe"); err == nil {
		t.Fatal("invalid UTF-8 must be rejected")
	}
}

func TestLuaIdent(t *testing.T) {
	if _, err := LuaIdent("egress_pool"); err != nil {
		t.Fatalf("valid identifier rejected: %v", err)
	}
	if _, err := LuaIdent("end"); err == nil {
		t.Fatal("reserved word must be rejected")
	}
	if _, err := LuaIdent("1abc"); err == nil {
		t.Fatal("leading digit must be rejected")
	}
	if _, err := LuaIdent("a-b"); err == nil {
		t.Fatal("hyphen must be rejected")
	}
}

func TestLintLuaCatchesSyntaxError(t *testing.T) {
	if issues := LintLua("local x = {"); len(issues) == 0 {
		t.Fatal("expected a lint issue for invalid Lua")
	}
	if issues := LintLua("local x = 1\nreturn x\n"); len(issues) != 0 {
		t.Fatalf("valid Lua should lint clean, got %v", issues)
	}
}

func TestInitChecksumDistinguishesReloadFromRestart(t *testing.T) {
	// The init checksum must change only for init-block changes (listeners, log
	// hook, spool) — which need a KumoMTA restart — and stay stable for
	// callback-only changes (VMTAs, suppressions, routing) which a reload picks up.
	base := func() ConfigSnapshot {
		return ConfigSnapshot{
			Listeners: []*Listener{{ID: "l1", Name: "edge", IPAddress: "203.0.113.1", Port: 2525, Hostname: "mx.example.com", Status: ListenerStatusActive}},
			VMTAs:     []*VMTA{{ID: "v1", Name: "vmta-a", ListenerID: "l1", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive}},
			Routes: []*RoutingRule{
				{ID: "r1", Name: "bulk", MatchType: MatchMailclass, MatchHeader: "X-Mail-Class", MatchValue: "bulk", Priority: 100, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
			},
			LogStreamRedisURL: "redis://redis:6379",
		}
	}
	initOf := func(s ConfigSnapshot) string {
		r, err := RenderKumoConfig(s)
		if err != nil || !r.Valid {
			t.Fatalf("render: err=%v valid=%v", err, r.Valid)
		}
		return r.InitChecksum
	}
	ref := initOf(base())

	// Callback-only changes → SAME init checksum (reload suffices).
	withVMTA := base()
	withVMTA.VMTAs = append(withVMTA.VMTAs, &VMTA{ID: "v2", Name: "vmta-b", ListenerID: "l1", IPAddress: "203.0.113.11", EHLOName: "b.example.com", Status: VMTAStatusActive})
	if initOf(withVMTA) != ref {
		t.Error("adding a VMTA must not change the init checksum (reload-safe)")
	}
	withSupp := base()
	withSupp.Suppressions = []*SuppressionEntry{{ID: "s1", Type: SuppressEmail, Value: "x@y.example", Status: SuppressActive}}
	if initOf(withSupp) != ref {
		t.Error("adding a suppression must not change the init checksum (reload-safe)")
	}
	withRcptRoute := base()
	withRcptRoute.Routes = append(withRcptRoute.Routes, &RoutingRule{ID: "r2", Name: "rcpt", MatchType: MatchRecipientDomain, MatchValue: "x.example", Priority: 50, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive})
	if initOf(withRcptRoute) != ref {
		t.Error("a recipient-domain route adds no log-hook header → init checksum must be stable")
	}

	// Init-block changes → DIFFERENT init checksum (restart required).
	withListener := base()
	withListener.Listeners = append(withListener.Listeners, &Listener{ID: "l2", Name: "submission", IPAddress: "203.0.113.2", Port: 2587, Hostname: "submit.example.com", Status: ListenerStatusActive})
	if initOf(withListener) == ref {
		t.Error("adding a listener must change the init checksum (restart required)")
	}
	withMailclassHeader := base()
	withMailclassHeader.Routes = append(withMailclassHeader.Routes, &RoutingRule{ID: "r3", Name: "promo", MatchType: MatchMailclass, MatchHeader: "X-Campaign-Type", MatchValue: "promo", Priority: 90, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive})
	if initOf(withMailclassHeader) == ref {
		t.Error("a new mailclass header changes the log-hook allow-list → init checksum must change (restart)")
	}
	noLogStream := base()
	noLogStream.LogStreamRedisURL = ""
	if initOf(noLogStream) == ref {
		t.Error("toggling the log hook must change the init checksum (restart required)")
	}
}

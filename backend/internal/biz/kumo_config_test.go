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

// TestRoutingHeaderVMTA verifies a header_vmta rule renders a headerless ROUTES
// entry and a select_pool branch that routes via the header value (guarded by
// HOSTED_DOMAINS + SOURCES) and strips the header.
func TestRoutingHeaderVMTA(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs:  []*VMTA{{ID: "v1", Name: "vmta-a", ListenerID: "l1", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive}},
		Groups: []*VMTAGroup{{ID: "g1", Name: "bulk-pool", Status: VMTAGroupStatusActive, Members: []VMTAGroupMember{{VMTAID: "v1", Weight: 100}}}},
		Routes: []*RoutingRule{
			{ID: "r1", Name: "by-header", MatchType: MatchHeaderVMTA, MatchHeader: "X-Kumo-VMTA", Priority: 200, Status: RoutingStatusActive},
		},
		DKIM: []*DKIMDomain{{ID: "d1", Domain: "example.com", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady}},
	}
	r, err := RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !r.Valid {
		t.Fatalf("policy failed lint: %v\n%s", r.LintIssues, r.Content)
	}
	if !strings.Contains(r.Content, `{ match_type = "header_vmta", match_header = "X-Kumo-VMTA", priority = 200 }`) {
		t.Errorf("missing header_vmta ROUTES entry:\n%s", r.Content)
	}
	for _, want := range []string{
		"route.match_type == 'header_vmta'",
		"msg:remove_all_named_headers(route.match_header)",
		"HOSTED_DOMAINS[domain] == nil and SOURCES[hv] ~= nil",
		"return hv",
	} {
		if !strings.Contains(r.Content, want) {
			t.Errorf("select_pool missing %q", want)
		}
	}
}

// TestRoutingMultiConditionOR verifies a mailclass rule with several
// header/value conditions renders one ROUTES row + MAIL_CLASSES entry per
// condition (all sharing the rule's pool/priority), giving OR match semantics.
func TestPerVMTATLSMode(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{
			{ID: "v1", Name: "secure-ip", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive, TLSMode: TLSModeRequired},
			{ID: "v2", Name: "plain-ip", IPAddress: "203.0.113.11", EHLOName: "b.example.com", Status: VMTAStatusActive},
		},
		DKIM: []*DKIMDomain{{ID: "d1", Domain: "example.com", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady}},
	}
	r, err := RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !r.Valid {
		t.Fatalf("policy failed lint: %v\n%s", r.LintIssues, r.Content)
	}
	if !strings.Contains(r.Content, `SOURCE_TLS["secure-ip"] = "Required"`) {
		t.Errorf("missing per-VMTA SOURCE_TLS entry:\n%s", r.Content)
	}
	if strings.Contains(r.Content, `SOURCE_TLS["plain-ip"]`) {
		t.Error("VMTA without a tls_mode should not emit a SOURCE_TLS entry")
	}
	// Domain policy must take precedence over the per-VMTA override.
	if !strings.Contains(r.Content, "REQUIRE_TLS_DOMAINS[string.lower(domain)] or SOURCE_TLS[egress_source]") {
		t.Errorf("egress path config missing domain>source TLS precedence:\n%s", r.Content)
	}
}

func TestVMTATLSModeValidation(t *testing.T) {
	base := func() *VMTA {
		return &VMTA{Name: "v", IPAddress: "203.0.113.1", EHLOName: "v.example.com"}
	}
	for _, m := range []string{"", TLSModeRequired, TLSModeRequiredInsecure, TLSModeOpportunisticInsecure, TLSModeDisabled} {
		v := base()
		v.TLSMode = m
		if err := v.Validate(); err != nil {
			t.Errorf("tls_mode %q should be valid: %v", m, err)
		}
	}
	v := base()
	v.TLSMode = "bogus"
	if err := v.Validate(); err == nil {
		t.Error("bogus tls_mode should be rejected")
	}
	// EnableTLSValue mapping.
	if (&VMTA{TLSMode: TLSModeRequired}).EnableTLSValue() != "Required" {
		t.Error("required → Required")
	}
	if (&VMTA{TLSMode: ""}).EnableTLSValue() != "" {
		t.Error("empty mode → no override")
	}
}

func TestRoutingMultiConditionOR(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs:  []*VMTA{{ID: "v1", Name: "vmta-a", ListenerID: "l1", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive}},
		Groups: []*VMTAGroup{{ID: "g1", Name: "bulk-pool", Status: VMTAGroupStatusActive, Members: []VMTAGroupMember{{VMTAID: "v1", Weight: 100}}}},
		Routes: []*RoutingRule{
			{ID: "r1", Name: "multi", MatchType: MatchMailclass, Priority: 100, TargetType: TargetVMTAGroup, TargetID: "g1", Status: RoutingStatusActive,
				MatchHeader: "X-Mail-Class", MatchValue: "bulk",
				Conditions: []RoutingMatchCondition{
					{Header: "X-Mail-Class", Value: "bulk"},
					{Header: "X-Mail-Class", Value: "promo"},
					{Header: "X-Campaign", Value: "spring"},
				}},
		},
		DKIM: []*DKIMDomain{{ID: "d1", Domain: "example.com", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady}},
	}
	r, err := RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !r.Valid {
		t.Fatalf("policy failed lint: %v\n%s", r.LintIssues, r.Content)
	}
	// One ROUTES row per condition, all pointing at bulk-pool.
	for _, want := range []string{
		`match_header = "X-Mail-Class", match_value = "bulk", priority = 100, egress_pool = "bulk-pool"`,
		`match_header = "X-Mail-Class", match_value = "promo", priority = 100, egress_pool = "bulk-pool"`,
		`match_header = "X-Campaign", match_value = "spring", priority = 100, egress_pool = "bulk-pool"`,
	} {
		if !strings.Contains(r.Content, want) {
			t.Errorf("missing ROUTES row: %s\n%s", want, r.Content)
		}
	}
	// RouteCount counts rules, not rows.
	if r.RouteCount != 1 {
		t.Errorf("RouteCount = %d, want 1 (one rule)", r.RouteCount)
	}
	// All three values classify (MAIL_CLASSES).
	for _, v := range []string{`["bulk"] = "bulk"`, `["promo"] = "promo"`, `["spring"] = "spring"`} {
		if !strings.Contains(r.Content, v) {
			t.Errorf("missing MAIL_CLASSES value: %s", v)
		}
	}
}

// TestHTTPInjectionHookOutbound verifies that HTTP-injected mail gets the same
// outbound processing as SMTP: mailclass classification, the VERP envelope
// rewrite, and egress-pool routing — not just DKIM signing.
func TestHTTPInjectionHookOutbound(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{
			{ID: "v1", Name: "vmta-a", ListenerID: "lst-1", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive},
		},
		Groups: []*VMTAGroup{
			{ID: "g1", Name: "bulk-pool", Status: VMTAGroupStatusActive, Members: []VMTAGroupMember{{VMTAID: "v1", Weight: 100}}},
		},
		Routes: []*RoutingRule{
			{ID: "r1", Name: "bulk", MatchType: MatchMailclass, MatchValue: "bulk", Priority: 100, TargetType: TargetVMTAGroup, TargetID: "g1", Status: RoutingStatusActive},
		},
		DKIM: []*DKIMDomain{
			{ID: "d1", Domain: "example.com", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady},
		},
		// Enable VERP: bounce domain + redis log stream + verp secret.
		BounceDomain:      "bounce.example.com",
		LogStreamRedisURL: "redis://localhost:6379",
		BounceVerpSecret:  "s3cret-verp-key",
	}
	r, err := RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !r.Valid {
		t.Fatalf("rendered policy failed lint: %v\n%s", r.LintIssues, r.Content)
	}
	// Isolate the http_message_generated hook body.
	start := strings.Index(r.Content, "kumo.on('http_message_generated'")
	if start < 0 {
		t.Fatalf("policy missing http_message_generated hook:\n%s", r.Content)
	}
	hook := r.Content[start:]
	if end := strings.Index(hook, "end)"); end >= 0 {
		hook = hook[:end]
	}
	for _, want := range []string{
		"classify_mail(msg)",           // mailclass classification
		"msg:set_meta('mailclass'",     // records the class
		"msg:set_sender(",              // VERP envelope rewrite
		"iris_bounce_domain(msg)",      // bounce-domain derivation
		"iris_dkim_sign(msg)",          // DKIM
		"select_pool(msg,",             // egress-pool routing
		"msg:set_meta('tenant', pool)", // records the pool
	} {
		if !strings.Contains(hook, want) {
			t.Errorf("http_message_generated hook missing %q; got:\n%s", want, hook)
		}
	}
}

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
		`kumo.configure_local_logs { log_dir = '/var/log/kumomta', max_segment_duration = '1 minute' }`,
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("generated init must contain %q:\n%s", want, r.Content)
		}
	}
}

func TestRenderEgressPinning(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs: []*VMTA{
			{ID: "v1", Name: "vmta-a", ListenerID: "l1", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive},
			{ID: "v2", Name: "vmta-b", ListenerID: "l1", IPAddress: "203.0.113.11", EHLOName: "b.example.com", Status: VMTAStatusActive},
		},
		Groups: []*VMTAGroup{
			{ID: "g1", Name: "bulk-pool", Status: VMTAGroupStatusActive, Members: []VMTAGroupMember{
				{VMTAID: "v1", Weight: 1}, {VMTAID: "v2", Weight: 1},
			}},
		},
		Routes: []*RoutingRule{
			{ID: "r1", Name: "bulk", MatchType: MatchMailclass, MatchValue: "bulk", Priority: 100, TargetType: TargetVMTAGroup, TargetID: "g1", Status: RoutingStatusActive},
		},
	}

	// Off (default): byte-for-byte free of any pinning artifacts.
	off, err := RenderKumoConfig(base)
	if err != nil || !off.Valid {
		t.Fatalf("render off: err=%v valid=%v", err, off.Valid)
	}
	if strings.Contains(off.Content, "iris_pin_egress_pool") || strings.Contains(off.Content, "-pin-") {
		t.Fatalf("pinning must not appear when disabled:\n%s", off.Content)
	}

	// On: still valid Lua, with the helper, the reception-hook call, and the
	// "<pool>-pin-<source>" resolution in get_egress_pool.
	on := base
	on.PinEgressPerMessage = true
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	for _, want := range []string{
		"local function iris_pin_egress_pool(pool, msg)",
		"pool = iris_pin_egress_pool(pool, msg)",
		`string.match(name, '.*%-pin%-(.+)$')`,
		`return pool .. '-pin-' .. e.name`,
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("pinning-on policy must contain %q:\n%s", want, r.Content)
		}
	}

	// The pinned pool name must NOT contain '@' or ':' — those are KumoMTA
	// queue-name delimiters (tenant@domain), and an '@' here corrupted the queue
	// name in v5.5.0 (regular-mails@vmta-04@domain → "Malformed label").
	if strings.Contains(r.Content, `.. '@' ..`) || strings.Contains(r.Content, `.. '@'..`) {
		t.Fatalf("pinned pool name must not join with '@' (collides with tenant@domain):\n%s", r.Content)
	}
}

func TestRenderChecksumIgnoresGeneratedBy(t *testing.T) {
	// The generated_by comment records who rendered the policy; it must NOT change
	// the checksum, or drift detection nags "changes pending" whenever a different
	// user regenerates an identical policy. The comment must still ship in Content.
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "l1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	}
	a := base
	a.GeneratedBy = "alice@example.com"
	b := base
	b.GeneratedBy = "bob@example.com"

	ra, err := RenderKumoConfig(a)
	if err != nil {
		t.Fatalf("render a: %v", err)
	}
	rb, err := RenderKumoConfig(b)
	if err != nil {
		t.Fatalf("render b: %v", err)
	}
	if ra.Checksum != rb.Checksum {
		t.Fatalf("checksum must ignore generated_by: %s != %s", ra.Checksum, rb.Checksum)
	}
	if ra.InitChecksum != rb.InitChecksum {
		t.Fatalf("init checksum must ignore generated_by")
	}
	// The audit comment still ships in the rendered content.
	if !strings.Contains(ra.Content, "-- generated_by = alice@example.com") {
		t.Fatalf("generated_by comment must remain in content:\n%s", ra.Content)
	}
	// A real policy change (an extra VMTA) MUST still change the checksum.
	c := base
	c.GeneratedBy = "alice@example.com"
	c.VMTAs = append(append([]*VMTA{}, base.VMTAs...),
		&VMTA{ID: "v2", Name: "v2", ListenerID: "l1", IPAddress: "203.0.113.2", EHLOName: "v2.example.com", Status: VMTAStatusActive})
	rc, err := RenderKumoConfig(c)
	if err != nil {
		t.Fatalf("render c: %v", err)
	}
	if rc.Checksum == ra.Checksum {
		t.Fatal("a real policy change must change the checksum")
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
		// Date is injected when absent, before signing, in both hooks — Date is in
		// the DKIM signed-header set, so an absent Date would otherwise break the
		// signature when a downstream MTA adds one (and trips rspamd MISSING_DATE).
		"local function iris_ensure_date(msg)",
		"msg:prepend_header('Date', os.date('!%a, %d %b %Y %H:%M:%S +0000'))",
		"iris_ensure_date(msg)",
		// Subdomain signing: a From of infra.example.com is signed by a example.com
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
	if !strings.Contains(off.Content, "local function iris_rspamd_scan(_msg, _enforce) end") {
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
		!strings.Contains(r.Content, "iris_rspamd_scan(msg, RSPAMD_ENFORCE)") {
		t.Fatalf("rspamd scan not wired into reception:\n%s", r.Content)
	}
	// The scan verdict is published to the results stream the ingestion worker
	// drains (the producer half of the Rspamd Results page).
	if !strings.Contains(r.Content, `RSPAMD_RESULTS_STREAM = "`+RspamdResultsStream+`"`) ||
		!strings.Contains(r.Content, "'XADD', RSPAMD_RESULTS_STREAM") {
		t.Fatalf("rspamd verdict not published to results stream:\n%s", r.Content)
	}
	// log hook: configure_log_hook in init + XADD constructor + tracker queue.
	if !strings.Contains(r.Content, "configure_log_hook") ||
		!strings.Contains(r.Content, "make.redis_tracker") ||
		!strings.Contains(r.Content, "'XADD'") ||
		!strings.Contains(r.Content, "domain == LOGSTREAM_TRACKER") {
		t.Fatalf("log hook not fully wired:\n%s", r.Content)
	}

	// rspamd enabled but no Redis stream: scanning still tags mail, but the
	// results producer must be omitted (no stream to write to).
	noRedis := base
	noRedis.RspamdMode = "tag"
	noRedis.RspamdURL = "http://rspamd:11334"
	nr, err := RenderKumoConfig(noRedis)
	if err != nil || !nr.Valid {
		t.Fatalf("render no-redis: err=%v valid=%v issues=%v", err, nr.Valid, nr.LintIssues)
	}
	if !strings.Contains(nr.Content, "iris_rspamd_scan(msg, RSPAMD_ENFORCE)") {
		t.Fatalf("rspamd scan must still run without redis:\n%s", nr.Content)
	}
	if strings.Contains(nr.Content, "RSPAMD_RESULTS_STREAM") {
		t.Fatalf("results producer must be absent without a redis stream:\n%s", nr.Content)
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
		// Non-Require-TLS domains encrypt opportunistically without hard-failing
		// on cert verification (legacy/retired chains must not defer delivery).
		"params.enable_tls = params.enable_tls or 'OpportunisticInsecure'",
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
	for _, want := range []string{"rset_timeout or '30s'", "idle_timeout or '60s'", "data_timeout or '60s'"} {
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
	if !strings.Contains(r.Content, "meta = { 'tenant', 'mailclass', 'node' }") {
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
		"msg:set_sender(string.format('b+%s.%s@%s', string.sub(tostring(mac), 1, 16), mid, iris_bounce_domain(msg)))",
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

func TestRenderWarmup(t *testing.T) {
	base := ConfigSnapshot{
		ShapingDir: "/test",
		VMTAs:      []*VMTA{{ID: "v1", Name: "warm-1", IPAddress: "203.0.113.1", EHLOName: "warm-1.example.com", Status: VMTAStatusActive}},
	}

	// The egress path loads the shaping sidecar files and merges iris's timeouts.
	r, err := RenderKumoConfig(base)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	for _, want := range []string{
		"local shaping = require 'policy-extras.shaping'",
		"shaping:setup_with_automation {",
		"no_default_files = true,",
		`extra_files = { "/test/iris-base.toml", "/test/iris-warmup.toml" },`,
		"local params = iris_shaper.get_egress_path_config(domain, egress_source, site_name, true)",
		"params.rset_timeout = params.rset_timeout or '30s'",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("shaping egress path missing %q:\n%s", want, r.Content)
		}
	}
	// TSA off by default: no publish/subscribe.
	if strings.Contains(r.Content, "publish = {") {
		t.Fatalf("TSA must be off without a TSA URL:\n%s", r.Content)
	}

	// With a TSA URL, publish/subscribe are emitted.
	tsa := base
	tsa.TSAUrl = "http://tsa:8008"
	rt, _ := RenderKumoConfig(tsa)
	if !strings.Contains(rt.Content, `publish = { "http://tsa:8008" }`) ||
		!strings.Contains(rt.Content, `subscribe = { "http://tsa:8008" }`) {
		t.Fatalf("TSA publish/subscribe not emitted:\n%s", rt.Content)
	}
	// The legacy MBP_BUCKET path must be gone.
	if strings.Contains(r.Content, "WARMUP_RATE") || strings.Contains(r.Content, "MBP_BUCKET") {
		t.Fatalf("legacy warmup tables must be retired:\n%s", r.Content)
	}

	// The warmup overrides are rendered into the sidecar TOML (not the policy Lua).
	on := base
	on.WarmupRates = map[string]map[string]string{"warm-1": {MBPGmail: "50/day"}}
	on.Blueprints = []*DeliveryBlueprint{{Provider: "Gmail", MXPattern: "google.com", ConnRate: "5/min", ConnLimit: 3, DailyCap: 150, Status: BlueprintActive}}
	r2, _ := RenderKumoConfig(on)
	if !strings.Contains(r2.ShapingWarmup, `["google.com".sources."warm-1"]`) ||
		!strings.Contains(r2.ShapingWarmup, `max_message_rate = "50/day"`) {
		t.Fatalf("warmup override not in sidecar TOML:\n%s", r2.ShapingWarmup)
	}
	if !strings.Contains(r2.ShapingBase, `["google.com"]`) {
		t.Fatalf("blueprint not in base TOML:\n%s", r2.ShapingBase)
	}
}

func TestBounceDomainTemplate(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs:             []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		BounceDomain:      "bounce.kumo.example.com",
		BounceVerpSecret:  "verp-key",
		LogStreamRedisURL: "redis://redis:6379", // bounce pipeline needs a stream
		DKIM: []*DKIMDomain{
			{ID: "d1", Domain: "example.com", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady},
			{ID: "d2", Domain: "economy.bg", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady},
		},
	}

	// No template: the per-domain maps render empty and VERP falls back to the
	// single global BOUNCE_DOMAIN — no behavior change from before the feature.
	off, err := RenderKumoConfig(base)
	if err != nil || !off.Valid {
		t.Fatalf("render off: err=%v valid=%v issues=%v", err, off.Valid, off.LintIssues)
	}
	if !strings.Contains(off.Content, "local BOUNCE_DOMAIN_BY_FROM = {}\n") ||
		!strings.Contains(off.Content, "local BOUNCE_DOMAINS = {}\n") {
		t.Fatalf("empty template must render empty per-domain maps:\n%s", off.Content)
	}
	// A populated entry is a quoted-key assignment (BOUNCE_DOMAIN_BY_FROM["x"]);
	// the helper body's BOUNCE_DOMAIN_BY_FROM[fdom] lookup must not count.
	if strings.Contains(off.Content, `BOUNCE_DOMAIN_BY_FROM["`) {
		t.Fatalf("empty template must not populate the per-domain map:\n%s", off.Content)
	}

	// Template set: derive bounce.kumo.<from> for each DKIM domain, build the
	// reverse inbound set, and select per-From-domain at VERP time.
	on := base
	on.BounceDomainTemplate = "bounce.kumo.{domain}"
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	for _, want := range []string{
		// Outbound: From-domain → aligned bounce domain.
		`BOUNCE_DOMAIN_BY_FROM["example.com"] = "bounce.kumo.example.com"`,
		`BOUNCE_DOMAIN_BY_FROM["economy.bg"] = "bounce.kumo.economy.bg"`,
		// Inbound: the reverse set used by the DSN router and listener.
		`BOUNCE_DOMAINS["bounce.kumo.example.com"] = true`,
		`BOUNCE_DOMAINS["bounce.kumo.economy.bg"] = true`,
		// Selection helper + its use in the VERP rewrite.
		"local function iris_bounce_domain(msg)",
		"mid, iris_bounce_domain(msg)))",
		// Inbound matchers accept the derived domains alongside BOUNCE_DOMAIN.
		"if rdom == BOUNCE_DOMAIN or BOUNCE_DOMAINS[rdom] then",
		"if (BOUNCE_DOMAIN ~= '' and domain == BOUNCE_DOMAIN) or BOUNCE_DOMAINS[domain] then",
	} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("bounce template missing %q:\n%s", want, r.Content)
		}
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
		InboundRoutes: []*InboundRoute{
			{ID: "w1", Name: "support", MatchType: MatchRecipientEmail, MatchValue: "support@server-lab.info",
				Action: InboundActionWebhook, DestinationURL: "https://portal.example/hook", SecretRef: "s3cr3t", Status: InboundRouteActive},
			{ID: "w2", Name: "dom", MatchType: MatchRecipientDomain, MatchValue: "leads.example.com",
				Action: InboundActionWebhook, DestinationURL: "https://portal.example/leads", Status: InboundRouteActive},
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
	if !strings.Contains(r.Content, `ROUTE_DOMAINS["server-lab.info"] = true`) ||
		!strings.Contains(r.Content, `ROUTE_DOMAINS["leads.example.com"] = true`) {
		t.Fatalf("ROUTE_DOMAINS not emitted:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, "if ROUTE_DOMAINS[domain] then") {
		t.Fatalf("get_listener_domain must relay route domains:\n%s", r.Content)
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
	// Reception routes matched mail to the action queue; queue config wires it.
	if !strings.Contains(r.Content, "msg:set_meta('queue', route.queue)") ||
		!strings.Contains(r.Content, "if domain == WEBHOOK_TRACKER then") {
		t.Fatalf("webhook reception/queue routing not wired:\n%s", r.Content)
	}
	// Route-captured mail is tagged with the action's class (here 'webhook').
	if !strings.Contains(r.Content, `ROUTE_BY_EMAIL["support@server-lab.info"] = { queue = "iris_webhook", class = "webhook", scan = "off" }`) {
		t.Fatalf("ROUTE_BY_EMAIL entry not emitted:\n%s", r.Content)
	}
	// A recipient at a relayed domain that matches no route is rejected so the
	// sending MTA bounces it, rather than relaying to the domain's real MX.
	if !strings.Contains(r.Content, "if ROUTE_DOMAINS[rdom] then") ||
		!strings.Contains(r.Content, "recipient rejected, no matching route") {
		t.Fatalf("unknown-recipient reject for route domains not emitted:\n%s", r.Content)
	}
}

func TestInboundMaildirAndForwardGeneration(t *testing.T) {
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		InboundRoutes: []*InboundRoute{
			{ID: "m1", Name: "store", MatchType: MatchRecipientDomain, MatchValue: "archive.example.com",
				Action: InboundActionMaildir, Status: InboundRouteActive},
			{ID: "m2", Name: "store-custom", MatchType: MatchRecipientEmail, MatchValue: "ceo@example.com",
				Action: InboundActionMaildir, MaildirPath: "/srv/mail/ceo", Status: InboundRouteActive},
			{ID: "f1", Name: "relay", MatchType: MatchRecipientDomain, MatchValue: "legacy.example.com",
				Action: InboundActionForward, ForwardHost: "mail.internal", ForwardPort: 2525, ForwardTLS: ForwardTLSRequired, Status: InboundRouteActive},
		},
		InboundMaildirBase: "/var/mail",
		LogStreamRedisURL:  "redis://redis:6379",
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	// Default maildir path uses the deployment base + per-user template.
	if !strings.Contains(r.Content, `MAILDIR_PATHS["iris_maildir_0"] = "/var/mail/{{ domain_part }}/{{ local_part }}"`) {
		t.Fatalf("default maildir path not emitted:\n%s", r.Content)
	}
	// Explicit per-route maildir path is honored as its own destination.
	if !strings.Contains(r.Content, `MAILDIR_PATHS["iris_maildir_1"] = "/srv/mail/ceo"`) {
		t.Fatalf("explicit maildir path not emitted:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, "maildir_path = MAILDIR_PATHS[domain]") {
		t.Fatalf("maildir queue config not wired:\n%s", r.Content)
	}
	// Forward pins the smarthost and requires TLS on its egress path.
	if !strings.Contains(r.Content, `FORWARD_SMARTHOSTS["iris_forward_0"] = { mx = "mail.internal:2525" }`) {
		t.Fatalf("forward smarthost not emitted:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, "mx_list = { fwd.mx }") {
		t.Fatalf("forward queue config not wired:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, `REQUIRE_TLS_DOMAINS["iris_forward_0"] = "Required"`) {
		t.Fatalf("forward require-TLS not emitted:\n%s", r.Content)
	}
	// Each recipient maps to its action queue.
	if !strings.Contains(r.Content, `ROUTE_BY_EMAIL["ceo@example.com"] = { queue = "iris_maildir_1", class = "maildir", scan = "off" }`) {
		t.Fatalf("maildir route entry not emitted:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, `ROUTE_BY_DOMAIN["legacy.example.com"] = { queue = "iris_forward_0", class = "forward", scan = "off" }`) {
		t.Fatalf("forward route entry not emitted:\n%s", r.Content)
	}
}

func TestInboundRoutesRspamdScanned(t *testing.T) {
	// Global mode enforce: a route with default scan resolves to enforce; the
	// route domain is hosted so the HOSTED_DOMAINS guard passes.
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		InboundRoutes: []*InboundRoute{
			{ID: "m1", Name: "store", MatchType: MatchRecipientDomain, MatchValue: "archive.example.com",
				Action: InboundActionMaildir, Status: InboundRouteActive, SpamScan: ScanDefault},
		},
		RspamdMode:        "enforce",
		RspamdURL:         "http://rspamd:11333",
		LogStreamRedisURL: "redis://redis:6379",
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	if !strings.Contains(r.Content, `HOSTED_DOMAINS["archive.example.com"] = true`) {
		t.Fatalf("route domain not added to HOSTED_DOMAINS:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, `ROUTE_BY_DOMAIN["archive.example.com"] = { queue = "iris_maildir_0", class = "maildir", scan = "enforce" }`) {
		t.Fatalf("default scan did not resolve to enforce from global mode:\n%s", r.Content)
	}
	// The route block dispatches the scan per route.scan before queueing.
	if !strings.Contains(r.Content, "if route.scan == 'enforce' then") ||
		!strings.Contains(r.Content, "iris_rspamd_scan(msg, true)") {
		t.Fatalf("route dispatch does not scan per route.scan:\n%s", r.Content)
	}
}

func TestInboundRoutePerRouteScanOverride(t *testing.T) {
	// Global mode off, but a route opts into tag scanning: the machinery is
	// emitted (URL is set) and that route resolves to tag while a default route
	// resolves to off.
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
		InboundRoutes: []*InboundRoute{
			{ID: "a", Name: "scan-me", MatchType: MatchRecipientDomain, MatchValue: "scan.example.com",
				Action: InboundActionMaildir, Status: InboundRouteActive, SpamScan: ScanTag},
			{ID: "b", Name: "no-scan", MatchType: MatchRecipientDomain, MatchValue: "plain.example.com",
				Action: InboundActionMaildir, Status: InboundRouteActive, SpamScan: ScanDefault},
		},
		RspamdMode:        "off",
		RspamdURL:         "http://rspamd:11333",
		LogStreamRedisURL: "redis://redis:6379",
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	// The opt-in route scans (tag); the default route follows the global off mode.
	if !strings.Contains(r.Content, `ROUTE_BY_DOMAIN["scan.example.com"] = { queue = "iris_maildir_0", class = "maildir", scan = "tag" }`) {
		t.Fatalf("per-route tag override not applied:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, `ROUTE_BY_DOMAIN["plain.example.com"] = { queue = "iris_maildir_0", class = "maildir", scan = "off" }`) {
		t.Fatalf("default route should resolve to off under global off mode:\n%s", r.Content)
	}
	// The scan machinery is emitted even though the global mode is off.
	if strings.Contains(r.Content, "local function iris_rspamd_scan(_msg, _enforce) end") {
		t.Fatalf("rspamd machinery should be enabled by the opt-in route:\n%s", r.Content)
	}
}

func TestDMARCCatcherGeneration(t *testing.T) {
	base := ConfigSnapshot{
		VMTAs: []*VMTA{{ID: "v1", Name: "v1", ListenerID: "l1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: VMTAStatusActive}},
	}

	// Enabled (report address + redis): catcher, tracker route, listener relay.
	on := base
	on.DMARCReportAddr = "dmarc@kmx.example.com"
	on.LogStreamRedisURL = "redis://redis:6379"
	r, err := RenderKumoConfig(on)
	if err != nil || !r.Valid {
		t.Fatalf("render on: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	for _, want := range []string{
		`local DMARC_REPORT_ADDR   = "dmarc@kmx.example.com"`,
		`local DMARC_REPORT_DOMAIN = "kmx.example.com"`,
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

func TestRenderCollectsListenerTLSFiles(t *testing.T) {
	snap := ConfigSnapshot{
		Listeners: []*Listener{
			// TLS-enabled: both cert and key paths are collected.
			{ID: "l1", Name: "mx", IPAddress: "203.0.113.1", Port: 25, Hostname: "mx.example.com", Status: ListenerStatusActive,
				TLSEnabled: true, TLSCertPath: "/etc/kumomta/certs/mx.pem", TLSKeyPath: "/etc/kumomta/certs/mx.key"},
			// Shares the same cert (multi-listener SNI): must dedupe, not duplicate.
			{ID: "l2", Name: "submission", IPAddress: "203.0.113.1", Port: 587, Hostname: "mx.example.com", Status: ListenerStatusActive,
				TLSEnabled: true, TLSCertPath: "/etc/kumomta/certs/mx.pem", TLSKeyPath: "/etc/kumomta/certs/mx.key"},
			// TLS-disabled: contributes nothing even if paths lingered.
			{ID: "l3", Name: "plain", IPAddress: "203.0.113.2", Port: 2525, Hostname: "mx.example.com", Status: ListenerStatusActive},
		},
	}
	r, err := RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got := make([]string, len(r.TLSFiles))
	for i, f := range r.TLSFiles {
		if f.Content != "" {
			t.Errorf("renderer must leave TLSFiles content empty (hydrated at apply time); got %q", f.Content)
		}
		got[i] = f.Path
	}
	want := []string{"/etc/kumomta/certs/mx.key", "/etc/kumomta/certs/mx.pem"} // sorted, deduped
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("TLS file paths = %v, want %v", got, want)
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

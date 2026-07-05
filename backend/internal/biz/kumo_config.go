package biz

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// ConfigSnapshot is the set of active Iris configuration entities used to render
// a KumoMTA policy. It is assembled from the repositories at generation time.
type ConfigSnapshot struct {
	Listeners    []*Listener
	VMTAs        []*VMTA
	Groups       []*VMTAGroup
	Routes       []*RoutingRule
	DKIM         []*DKIMDomain
	Suppressions []*SuppressionEntry
	// TLSPolicies require TLS on outbound delivery to matched destination
	// domains (enable_tls=Required on the egress path). Inactive entries are
	// skipped at render time.
	TLSPolicies []*TLSPolicy

	// Blueprints are the active base shaping rules (per provider/MX pattern)
	// rendered into the base shaping config.
	Blueprints []*DeliveryBlueprint

	// AutomationRules are the active operator-authored TSA automation rules
	// rendered into iris-automation.toml (loaded by the TSA daemon).
	AutomationRules []*AutomationRule

	// ShapingDir is the directory the policy loads iris-base.toml + iris-warmup.toml
	// from (written next to the policy by the apply adapter). Empty falls back to
	// the standard policy dir.
	ShapingDir string

	// TSAUrl, when set, is the KumoMTA Traffic Shaping Automation daemon URL the
	// policy publishes log events to and subscribes to for adaptive (hourly)
	// back-off. Empty disables TSA (static shaping only).
	TSAUrl string

	// WarmupSchedules are the active/paused IP-warmup schedules loaded for the
	// policy; the render step resolves them to WarmupRates for the current date.
	WarmupSchedules []*WarmupSchedule

	// WarmupRates carries the per-egress-source, per-MBP-bucket message-rate caps
	// for IP warmup, as KumoMTA throttle specs ("N/day"): WarmupRates[vmtaName]
	// [bucket] = rate. Resolved from WarmupSchedules for the current date in the
	// render step (ResolveWarmupRates), so RenderKumoConfig stays a pure
	// snapshot→policy function. Empty when no warmup is in effect.
	WarmupRates map[string]map[string]string
	// InboundRoutes are active inbound routes (maildir / forward / webhook). Their
	// recipient domains are relay-accepted by get_listener_domain, and matching
	// inbound mail is dispatched to the action's queue: a Maildir on disk, a
	// pinned forwarding smarthost, or the in-policy webhook poster
	// (make.webhook_post) which forwards the raw message to the destination URL.
	// Webhook routes include the HMAC secret (SecretRef).
	InboundRoutes []*InboundRoute

	// InboundMaildirBase is the deployment-wide Maildir root. A maildir route with
	// an empty MaildirPath lands under InboundMaildirBase/<domain>/<local-part>.
	InboundMaildirBase string
	// EgressEHLODefault is the default outbound EHLO hostname applied to any
	// egress source that does not set its own.
	EgressEHLODefault string

	// RspamdMode enables inbound spam filtering in the generated policy:
	// "" / "off" (disabled), "tag" (scan + X-Spam headers, never reject), or
	// "enforce" (honor rspamd's reject/greylist verdict). RspamdURL is the
	// rspamd HTTP endpoint. Only mail received for HostedDomains is scanned.
	RspamdMode string
	RspamdURL  string
	// HostedDomains is the set of recipient domains this deployment hosts
	// (used to scope rspamd to inbound mail). When empty it is derived from
	// the configured DKIM domains.
	HostedDomains []string

	// LogStreamRedisURL, when set, makes the policy stream KumoMTA's structured
	// log records (Reception/Delivery/Bounce/AdminBounce/Expiration/
	// TransientFailure/Feedback) into a
	// Redis stream via a log_hook; the Iris log consumer ingests them into the
	// mail_records hypertable. LogStreamName is the stream name.
	LogStreamRedisURL string
	LogStreamName     string

	// EsmtpListen / HTTPListen are the default listener bind specs emitted in
	// the policy init so the rendered config is self-contained.
	EsmtpListen string
	HTTPListen  string

	// Delivery rates: the outbound retry schedule applied in get_queue_config.
	EgressRetryInterval    string
	EgressMaxRetryInterval string
	EgressMaxAge           string

	// BounceDomain, when set, enables the DSN catcher: inbound mail to this
	// domain is routed to the DSN Redis stream (and the domain is relayed by
	// every listener) for the bounce consumer to process.
	BounceDomain string

	// BounceDomainTemplate, when set, derives a per-sending-domain bounce (VERP
	// return-path) domain by substituting BounceDomainPlaceholder with each DKIM
	// (sending) domain — e.g. "bounce.kumo.{domain}" makes mail from @example.com
	// use @bounce.kumo.example.com, aligning SPF with the From-domain. Empty uses
	// BounceDomain for all mail. The derived domains are also accepted inbound
	// (relayed to the DSN catcher) alongside BounceDomain.
	BounceDomainTemplate string

	// DMARCReportAddr, when set, enables the DMARC catcher: inbound mail to this
	// exact address is routed to the DMARC Redis stream (and its domain relayed)
	// for the report parser to consume.
	DMARCReportAddr string

	// BounceClassifierFile, when set, makes the init block load KumoMTA's bounce
	// classifier rules so Bounce log records carry a classification category.
	BounceClassifierFile string

	// BounceVerpSecret, when set alongside a bounce domain, makes the policy
	// rewrite the outbound envelope sender to a VERP return-path
	// (b+<hmac>.<msgid>@<bounce_domain>) so async DSNs correlate to the message.
	BounceVerpSecret string

	// FBLEndpoints are the per-domain feedback-loop enrollments. An "approved"
	// endpoint makes kumod parse RFC 5965 ARF feedback reports at its domain
	// (log_arf) and emit a Feedback log record, which the log hook streams to the
	// feedback consumer (auto-suppression; requires LogStreamRedisURL). An
	// "awaiting_approval" endpoint instead relays its domain and forwards mail
	// arriving at its feedback address to the forward address (so a human can
	// read the mailbox provider's enrollment-confirmation email).
	FBLEndpoints []*FBLEndpoint

	// GeneratedBy/GeneratorVersion annotate the rendered header.
	GeneratedBy      string
	GeneratorVersion string
}

// rspamdEnabled reports whether inbound rspamd scanning should be rendered.
func (s ConfigSnapshot) rspamdEnabled() bool {
	m := strings.ToLower(strings.TrimSpace(s.RspamdMode))
	return (m == "tag" || m == "enforce") && strings.TrimSpace(s.RspamdURL) != ""
}

// rspamdEnforce reports whether rspamd verdicts are enforced (vs tag-only).
func (s ConfigSnapshot) rspamdEnforce() bool {
	return strings.EqualFold(strings.TrimSpace(s.RspamdMode), "enforce")
}

// hostedDomains returns the scoping set for rspamd, deriving it from DKIM
// domains when not explicitly provided.
func (s ConfigSnapshot) hostedDomains() []string {
	if len(s.HostedDomains) > 0 {
		return s.HostedDomains
	}
	seen := map[string]struct{}{}
	var out []string
	for _, d := range s.DKIM {
		dom := strings.ToLower(strings.TrimSpace(d.Domain))
		if dom == "" {
			continue
		}
		if _, ok := seen[dom]; ok {
			continue
		}
		seen[dom] = struct{}{}
		out = append(out, dom)
	}
	sort.Strings(out)
	return out
}

// RenderedConfig is a generated KumoMTA policy plus a summary of its contents
// and the result of linting it.
type RenderedConfig struct {
	Content  string
	Checksum string
	// InitChecksum is a hash of just the kumo.on('init') block. Because KumoMTA
	// only runs init at startup, a change here requires a restart, not a reload.
	InitChecksum     string
	VMTACount        int
	PoolCount        int
	RouteCount       int
	DKIMCount        int
	SuppressionCount int
	// Valid is true when the rendered policy passed the Lua syntax lint.
	Valid bool
	// LintIssues holds any syntax problems found by the linter.
	LintIssues []string
	// ShapingBase / ShapingWarmup are the TOML sidecar files the policy loads via
	// kumo.shaping.load when ShapingDir is configured (base blueprints + per-IP
	// warmup overrides). ShapingAutomation holds the TSA automation rules loaded
	// by the TSA daemon. The apply adapter writes all three next to the policy.
	ShapingBase       string
	ShapingWarmup     string
	ShapingAutomation string
}

// validateSnapshot re-runs model validation on every entity so the renderer
// never interpolates an unvalidated value. This is the render-gate: callers
// may have built the snapshot from the database, but we re-check here so a
// programming error upstream cannot bypass validation.
func validateSnapshot(snap ConfigSnapshot) error {
	for _, l := range snap.Listeners {
		if err := l.Validate(); err != nil {
			return Invalid("CONFIG_LISTENER_INVALID", "listener %q: %v", l.Name, err)
		}
	}
	for _, v := range snap.VMTAs {
		if err := v.Validate(); err != nil {
			return Invalid("CONFIG_VMTA_INVALID", "vmta %q: %v", v.Name, err)
		}
	}
	for _, g := range snap.Groups {
		if err := g.Validate(); err != nil {
			return Invalid("CONFIG_GROUP_INVALID", "group %q: %v", g.Name, err)
		}
	}
	for _, r := range snap.Routes {
		if err := r.Validate(); err != nil {
			return Invalid("CONFIG_ROUTE_INVALID", "route %q: %v", r.Name, err)
		}
	}
	for _, d := range snap.DKIM {
		if err := d.Validate(); err != nil {
			return Invalid("CONFIG_DKIM_INVALID", "dkim %q: %v", d.Domain, err)
		}
	}
	for _, s := range snap.Suppressions {
		if err := s.Validate(); err != nil {
			return Invalid("CONFIG_SUPPRESSION_INVALID", "suppression %q: %v", s.Value, err)
		}
	}
	return nil
}

// RenderKumoConfig translates the configuration snapshot into a KumoMTA policy
// (Lua) that binds the Iris model to KumoMTA's real callback API:
//
//   - VMTAs become egress sources (get_egress_source) plus a single-member
//     egress pool of the same name, so a route targeting a VMTA resolves
//     through the same get_egress_pool callback as a group.
//   - VMTA groups become weighted egress pools.
//   - Routing rules set the `tenant` meta at reception; get_queue_config maps
//     that tenant to an egress pool.
//   - DKIM identities register signers attached at smtp_client_message_sending.
//   - Active suppressions are rejected at reception (smtp_server_message_received).
//
// The output is linted for Lua syntax validity before it is returned.
func RenderKumoConfig(snap ConfigSnapshot) (out RenderedConfig, err error) {
	if verr := validateSnapshot(snap); verr != nil {
		return RenderedConfig{}, verr
	}
	// Defensive: MustLuaString panics only on unvalidated unsafe input; convert
	// any such panic into a clean render error rather than crashing the server.
	defer func() {
		if r := recover(); r != nil {
			err = Internal(fmt.Errorf("%v", r), "render kumomta policy")
		}
	}()

	vmtaName := make(map[string]string, len(snap.VMTAs))
	for _, v := range snap.VMTAs {
		vmtaName[v.ID] = v.Name
	}
	groupName := make(map[string]string, len(snap.Groups))
	for _, g := range snap.Groups {
		groupName[g.ID] = g.Name
	}

	var b strings.Builder
	b.Grow(8 * 1024)

	// Header.
	ver := snap.GeneratorVersion
	if ver == "" {
		ver = "iris/0.1.0"
	}
	fmt.Fprintf(&b, "-- kumomta policy generated by %s. DO NOT EDIT BY HAND.\n", sanitizeComment(ver))
	if snap.GeneratedBy != "" {
		fmt.Fprintf(&b, "-- generated_by = %s\n", sanitizeComment(snap.GeneratedBy))
	}
	b.WriteString("local kumo = require 'kumo'\n\n")

	// Log-stream + bounce constants, init block (listeners + log hook).
	writeLogStreamConsts(&b, snap)
	writeBounceConsts(&b, snap)
	// Capture the init block separately: kumo.on('init') runs only once at
	// startup, so a hot reload does NOT pick up changes to it (listeners, spool,
	// log hook). Its own checksum lets Apply decide reload vs restart.
	var initBuf strings.Builder
	writeInit(&initBuf, snap)
	initContent := initBuf.String()
	b.WriteString(initContent)
	// Log-hook callbacks that XADD records into the Redis stream.
	writeLogHook(&b, snap)
	// Inbound webhook tables + poster. Emitted before the listener-domain and
	// reception hooks so they can reference the WEBHOOK_* locals as upvalues.
	writeInboundRoutes(&b, snap)
	// Listener-domain handler (bounce relay + FBL ARF parsing + webhook relay)
	// and the DSN XADD constructor.
	writeListenerDomain(&b, snap)
	writeDsnCatcher(&b, snap)
	writeDMARCCatcher(&b, snap)
	writeFBLSink(&b, snap)
	// VERP envelope rewrite (outbound return-path → bounce domain).
	writeBounceVerp(&b, snap)
	// Egress sources (one per active VMTA).
	rendered := writeEgressSources(&b, snap.VMTAs, snap.EgressEHLODefault)
	// Egress pools: a singleton pool per VMTA + one per active group.
	pools := writeEgressPools(&b, snap.VMTAs, snap.Groups, vmtaName)
	// Per-VMTA connection limits (max_connections) via the egress path config.
	fwdTLS, _ := forwardTargets(snap)
	writeEgressPaths(&b, snap.VMTAs, snap.TLSPolicies, fwdTLS, snap.ShapingDir, snap.TSAUrl)
	// DKIM signers.
	dkim := writeDKIMTable(&b, snap.DKIM)
	// DKIM signing function + the http-injection signing hook. Defined before the
	// reception hook so that hook can call iris_dkim_sign as an in-scope upvalue.
	writeDKIMSigning(&b)
	// Hosted domains + inbound rspamd scanning.
	writeHostedDomains(&b, snap)
	writeRspamd(&b, snap)
	// Suppression lookup (redis-backed; the list is no longer rendered inline).
	writeSuppression(&b, snap)
	// Routing table (priority-ordered) and reception hook (which signs DKIM).
	routes := writeRouting(&b, snap.Routes, vmtaName, groupName, snap.rspamdEnabled(), bounceEnabled(snap), inboundRoutesEnabled(snap), verpEnabled(snap), fblForwardEnabled(snap), dmarcEnabled(snap))
	// get_queue_config maps tenant → egress pool (and the log-stream tracker).
	writeQueueConfig(&b, snap)

	content := b.String()
	sum := sha256.Sum256([]byte(content))
	initSum := sha256.Sum256([]byte(initContent))

	issues := LintLua(content)
	return RenderedConfig{
		Content:          content,
		Checksum:         hex.EncodeToString(sum[:]),
		InitChecksum:     hex.EncodeToString(initSum[:]),
		VMTACount:        rendered,
		PoolCount:        pools,
		RouteCount:       routes,
		DKIMCount:        dkim,
		SuppressionCount: 0, // suppressions live in Redis now, not the config
		Valid:            len(issues) == 0,
		LintIssues:       issues,
		// Shaping sidecar files (written next to the policy by the apply adapter
		// when ShapingDir is set; the policy loads them via kumo.shaping.load).
		ShapingBase:       RenderBaseShaping(snap.Blueprints),
		ShapingWarmup:     RenderWarmupShaping(snap.WarmupRates),
		ShapingAutomation: RenderAutomation(snap.AutomationRules),
	}, nil
}

func writeEgressSources(b *strings.Builder, vmtas []*VMTA, ehloDefault string) int {
	b.WriteString("-- ===== egress sources (one per VMTA) =====\n")
	fmt.Fprintf(b, "local EGRESS_EHLO_DEFAULT = %s\n", MustLuaString(strings.TrimSpace(ehloDefault)))
	b.WriteString("local SOURCES = {}\n")
	n := 0
	for _, v := range sortedVMTAs(vmtas) {
		if v.Status != VMTAStatusActive && v.Status != VMTAStatusDraining {
			continue
		}
		fmt.Fprintf(b, "SOURCES[%s] = { source_address = %s, ehlo_domain = %s }\n",
			MustLuaString(v.Name), MustLuaString(v.IPAddress), MustLuaString(v.EHLOName))
		n++
	}
	b.WriteString(`
kumo.on('get_egress_source', function(name)
  local cfg = SOURCES[name] or {}
  local clean = { name = name, source_address = cfg.source_address, ehlo_domain = cfg.ehlo_domain }
  if (clean.ehlo_domain == nil or clean.ehlo_domain == '') and EGRESS_EHLO_DEFAULT ~= '' then
    clean.ehlo_domain = EGRESS_EHLO_DEFAULT
  end
  return kumo.make_egress_source(clean)
end)

`)
	return n
}

func writeEgressPools(b *strings.Builder, vmtas []*VMTA, groups []*VMTAGroup, vmtaName map[string]string) int {
	b.WriteString("-- ===== egress pools (one per VMTA + one per group) =====\n")
	b.WriteString("local POOLS = {}\n")
	for _, v := range sortedVMTAs(vmtas) {
		if v.Status != VMTAStatusActive && v.Status != VMTAStatusDraining {
			continue
		}
		fmt.Fprintf(b, "POOLS[%s] = { entries = { { name = %s } } }\n",
			MustLuaString(v.Name), MustLuaString(v.Name))
	}
	// Only active VMTAs are selectable for NEW mail via a group pool. A draining
	// VMTA is dropped from group entries (so weighting stops routing new mail to
	// it) but keeps its own singleton source/pool/path above, so already-queued
	// mail can still drain out before it is removed.
	selectable := make(map[string]bool, len(vmtas))
	for _, v := range vmtas {
		if v.Status == VMTAStatusActive {
			selectable[v.ID] = true
		}
	}
	n := 0
	for _, g := range sortedGroups(groups) {
		if g.Status != VMTAGroupStatusActive {
			continue
		}
		var entries []string
		for _, m := range g.Members {
			name, ok := vmtaName[m.VMTAID]
			if !ok || m.Weight <= 0 || !selectable[m.VMTAID] {
				continue
			}
			entries = append(entries, fmt.Sprintf("{ name = %s, weight = %d }", MustLuaString(name), m.Weight))
		}
		if len(entries) == 0 {
			continue
		}
		fmt.Fprintf(b, "POOLS[%s] = { entries = { %s } }\n", MustLuaString(g.Name), strings.Join(entries, ", "))
		n++
	}
	b.WriteString(`
kumo.on('get_egress_pool', function(name)
  local cfg = POOLS[name]
  if not cfg or not cfg.entries or #cfg.entries == 0 then
    return kumo.make_egress_pool { name = name, entries = { { name = name } } }
  end
  return kumo.make_egress_pool { name = name, entries = cfg.entries }
end)

`)
	return n
}

func writeDKIMTable(b *strings.Builder, ids []*DKIMDomain) int {
	b.WriteString("-- ===== dkim signers =====\n")
	b.WriteString("local DKIM_BY_DOMAIN = {}\n")
	n := 0
	for _, d := range sortedDKIM(ids) {
		// Only ready domains with key material sign; a ready domain without a key
		// would render an invalid signer.
		if d.Status != DKIMReady || strings.TrimSpace(d.PrivateKeyRef) == "" {
			continue
		}
		algo := "sha256"
		if strings.HasPrefix(strings.ToLower(d.Selector), "ed25519") {
			algo = "ed25519"
		}
		// The private key is supplied inline as KeySource key_data (PEM).
		fmt.Fprintf(b, "DKIM_BY_DOMAIN[%s] = { selector = %s, key = { key_data = %s }, algo = %s }\n",
			MustLuaString(d.Domain), MustLuaString(d.Selector), MustLuaString(d.PrivateKeyRef), MustLuaString(algo))
		n++
	}
	b.WriteString("\n")
	return n
}

// writeSuppression emits the is_suppressed lookup. The list itself lives in
// Redis (keys supp:e:<email> / supp:d:<domain> with a per-entry TTL), not in the
// config — so a 5M-send / 0.5%-bounce workload no longer bloats the policy and a
// new suppression takes effect within the memoize TTL without a config apply.
// The lookup uses the same Redis as the log stream; when that is unset,
// suppression enforcement is disabled (is_suppressed always false), the same
// degraded mode as the log hook.
func writeSuppression(b *strings.Builder, snap ConfigSnapshot) {
	b.WriteString("-- ===== suppression list (redis-backed, memoized) =====\n")
	if snap.LogStreamRedisURL == "" {
		b.WriteString("local function is_suppressed(_recipient) return false end\n\n")
		return
	}
	// _supp_lookup hits Redis (EXISTS on the exact email + its domain); memoize
	// caches the result briefly so repeat/retry recipients don't re-query. The
	// short TTL bounds how long a freshly-suppressed address can still get through
	// (negative results are cached too). Fail-open: a Redis error never blocks mail.
	b.WriteString(`local function _supp_lookup(recipient)
  local ok, res = pcall(function()
    local domain = recipient:match('@(.+)$') or ''
    local redis = require 'redis'
    local conn = redis.open { node = LOGSTREAM_REDIS_URL, pool_size = 10 }
    local n = conn:query('EXISTS', 'supp:e:' .. recipient, 'supp:d:' .. domain)
    return (tonumber(n) or 0) > 0
  end)
  if not ok then
    kumo.log_error('suppression: redis lookup failed: ' .. tostring(res))
    return false
  end
  return res
end

local is_suppressed = kumo.memoize(_supp_lookup, {
  name = 'iris_suppression',
  ttl = '60 seconds',
  capacity = 100000,
  allow_stale_reads = true,
})

`)
}

func writeRouting(b *strings.Builder, routes []*RoutingRule, vmtaName, groupName map[string]string, rspamd, bounce, inboundRoutes, verp, fblForward, dmarc bool) int {
	b.WriteString("-- ===== routing rules (descending priority: higher priority wins) =====\n")
	b.WriteString("-- A mailclass match is a header (name + value) pair; recipient matches use\n")
	b.WriteString("-- the address/domain. The first matching rule (highest priority) wins.\n")
	b.WriteString("local ROUTES = {\n")
	n := 0
	for _, r := range sortedRoutes(routes) {
		if r.Status != RoutingStatusActive {
			continue
		}
		var target string
		switch r.TargetType {
		case TargetVMTAGroup:
			target = groupName[r.TargetID]
		case TargetVMTA:
			target = vmtaName[r.TargetID]
		}
		if target == "" {
			continue
		}
		header := r.MatchHeader
		if r.MatchType == MatchMailclass && header == "" {
			header = DefaultMailClassHeader
		}
		fmt.Fprintf(b, "  { match_type = %s, match_header = %s, match_value = %s, priority = %d, egress_pool = %s },\n",
			MustLuaString(r.MatchType), MustLuaString(header), MustLuaString(r.MatchValue), r.Priority, MustLuaString(target))
		n++
	}
	b.WriteString("}\n")

	// Mailclass definitions, keyed by header → { value → class }. This drives
	// classification (what class a mail IS) independent of routing (where it
	// goes), so the Logs UI can show the class even when a recipient rule wins.
	// The class label is the matched header value.
	b.WriteString(`
-- MAIL_CLASSES maps a header name to the set of values that identify a class.
local MAIL_CLASSES = {
`)
	type hv struct{ header, value string }
	seenHV := map[hv]struct{}{}
	headers := map[string][]string{}
	var headerOrder []string
	for _, r := range sortedRoutes(routes) {
		if r.Status != RoutingStatusActive || r.MatchType != MatchMailclass {
			continue
		}
		header := r.MatchHeader
		if header == "" {
			header = DefaultMailClassHeader
		}
		key := hv{header, r.MatchValue}
		if _, ok := seenHV[key]; ok {
			continue
		}
		seenHV[key] = struct{}{}
		if _, ok := headers[header]; !ok {
			headerOrder = append(headerOrder, header)
		}
		headers[header] = append(headers[header], r.MatchValue)
	}
	sort.Strings(headerOrder)
	for _, h := range headerOrder {
		vals := headers[h]
		sort.Strings(vals)
		fmt.Fprintf(b, "  [%s] = {", MustLuaString(h))
		for _, v := range vals {
			fmt.Fprintf(b, " [%s] = %s,", MustLuaString(v), MustLuaString(v))
		}
		b.WriteString(" },\n")
	}
	b.WriteString(`}

-- classify_mail returns the class of a message from its configured mailclass
-- headers, or nil. Used to tag the 'mailclass' meta (logged for the Logs UI).
local function classify_mail(msg)
  for header, values in pairs(MAIL_CLASSES) do
    local hv = msg:get_first_named_header_value(header)
    if hv ~= nil and values[hv] ~= nil then return values[hv] end
  end
  return nil
end
`)

	writeSenderIPClasses(b, routes)

	b.WriteString(`
-- select_pool walks ROUTES (ordered by descending priority) and returns the
-- egress pool of the first matching rule. A mailclass rule matches its header
-- value OR the message's already-resolved class (e.g. one assigned by a
-- sender_ip rule); recipient rules match the envelope recipient / its domain.
local function select_pool(msg, recipient, class)
  local domain = recipient:match('@(.+)$') or ''
  for _, route in ipairs(ROUTES) do
    local matched = false
    if route.match_type == 'mailclass' then
      local hv = msg:get_first_named_header_value(route.match_header)
      matched = (hv ~= nil and hv == route.match_value) or (class ~= nil and class == route.match_value)
    elseif route.match_type == 'recipient_email' then
      matched = (route.match_value == recipient)
    elseif route.match_type == 'recipient_domain' then
      matched = (route.match_value == domain)
    end
    if matched then return route.egress_pool end
  end
  return nil
end

-- iris_log_suppressed streams a synthetic 'Suppressed' record onto the log
-- stream so a recipient rejected by the suppression list still appears in the
-- mail log (kumo.reject otherwise emits no record the Logs UI ingests). No-op
-- when the log stream is disabled; fail-open so a Redis hiccup never blocks the
-- reject.
local function iris_log_suppressed(msg, recipient)
  if not LOGSTREAM_REDIS_URL or LOGSTREAM_REDIS_URL == '' then return end
  local ok, err = pcall(function()
    local sender = ''
    local s = msg:sender()
    if s then sender = s.email or '' end
    local payload = kumo.serde.json_encode {
      type = 'Suppressed',
      id = tostring(msg:id()),
      sender = sender,
      recipient = recipient,
      headers = { From = msg:get_first_named_header_value('From') or '' },
    }
    local redis = require 'redis'
    local conn = redis.open { node = LOGSTREAM_REDIS_URL, pool_size = 5 }
    conn:query('XADD', LOGSTREAM_NAME, 'MAXLEN', '~', LOGSTREAM_MAXLEN, '*',
               'type', 'Suppressed', 'data', payload)
  end)
  if not ok then
    kumo.log_error('logstream: suppressed xadd failed err=' .. tostring(err))
  end
end

-- Reception hook: optionally scan with rspamd, reject suppressed recipients,
-- classify the mail (the 'mailclass' meta, logged for the Logs UI), then choose
-- an egress pool and record it as the 'tenant' meta for get_queue_config.
kumo.on('smtp_server_message_received', function(msg)
`)
	if bounce {
		// Inbound DSN catcher: mail arriving at the bounce domain is funneled
		// straight to the DSN tracker queue (before suppression, so a suppressed
		// recipient can't trigger a reject → bounce loop).
		b.WriteString(`  do
    local rcpt = msg:recipient()
    local rdom = (rcpt and rcpt.domain or ''):lower()
    if rdom == BOUNCE_DOMAIN or BOUNCE_DOMAINS[rdom] then
      msg:set_meta('queue', DSN_TRACKER)
      return
    end
  end
`)
	}
	if dmarc {
		// Inbound DMARC aggregate-report catcher: mail to the configured report
		// address is funneled to the DMARC tracker queue (XADDed to Redis for the
		// report parser), before suppression/classification.
		b.WriteString(`  do
    local rcpt = msg:recipient()
    if (rcpt and rcpt.email or ''):lower() == DMARC_REPORT_ADDR then
      msg:set_meta('queue', DMARC_TRACKER)
      return
    end
  end
`)
	}
	if fblForward {
		// Awaiting-approval FBL forward: mail arriving at a feedback address whose
		// endpoint is still awaiting approval is forwarded to the human approval
		// mailbox by rewriting the recipient and letting it relay onward (before
		// suppression/classification, so the mailbox provider's enrollment
		// confirmation is never dropped).
		b.WriteString(`  do
    local rcpt = msg:recipient()
    local email = (rcpt and rcpt.email or ''):lower()
    local fwd = FBL_FORWARD[email]
    if fwd then
      -- Forward by re-injecting a NEW, locally-originated message. Rewriting this
      -- inbound message's recipient and relaying it would be rejected as relaying
      -- to an external domain (anti-open-relay, since the sender is external); an
      -- injected message is treated as a local submission and is not relay-bound.
      -- The envelope sender is rewritten to a local address at the feedback domain
      -- so the forward passes SPF from our egress IP; it is pinned to a real
      -- egress source and tagged for the mail log. The original carrier message is
      -- then consumed by the sink queue.
      local dom = (rcpt and rcpt.domain or ''):lower()
      local ok, err = pcall(function()
        local copy = kumo.make_message('fbl-forward@' .. dom, fwd, msg:get_data())
        copy:set_meta('mailclass', 'fbl-forward')
        if FBL_FORWARD_POOL ~= '' then
          copy:set_meta('tenant', FBL_FORWARD_POOL)
        end
        kumo.inject_message(copy)
      end)
      if not ok then
        kumo.log_error('fbl: forward inject failed err=' .. tostring(err))
      end
      msg:set_meta('queue', FBL_FORWARD_SINK)
      return
    end
  end
`)
	}
	if inboundRoutes {
		// Inbound route dispatch: recipient-matched mail is routed (before
		// suppression/classification) to its action queue — the webhook poster, a
		// maildir destination, or a forwarding smarthost — instead of being relayed
		// onward. An exact email match outranks a domain match.
		b.WriteString(`  do
    local rcpt = msg:recipient()
    local email = (rcpt and rcpt.email or ''):lower()
    local rdom = (rcpt and rcpt.domain or ''):lower()
    local route = ROUTE_BY_EMAIL[email] or ROUTE_BY_DOMAIN[rdom]
    if route then
      -- Per-route spam scanning (resolved at render time to off/tag/enforce). In
      -- tag mode the X-Spam headers ride into the maildir / forwarded message /
      -- webhook body; in enforce mode a spam verdict rejects here (kumo.reject
      -- aborts the hook) before the message is stored or relayed. Route domains
      -- are in HOSTED_DOMAINS so the scan runs.
      if route.scan == 'enforce' then
        iris_rspamd_scan(msg, true)
      elseif route.scan == 'tag' then
        iris_rspamd_scan(msg, false)
      end
      -- Tag the class so route-captured mail is identifiable in the mail log
      -- (this hook returns before classify_mail runs).
      msg:set_meta('mailclass', route.class)
      msg:set_meta('queue', route.queue)
      return
    end
    if ROUTE_DOMAINS[rdom] then
      -- The domain is accepted for relay only because an inbound route exists for
      -- it, but this recipient matched no route above. Reject so the sending MTA
      -- issues the bounce to the originator, instead of relaying to the domain's
      -- real MX (which would fail and be swallowed by the VERP bounce path).
      kumo.reject(550, string.format('5.1.1 <%s>: recipient rejected, no matching route', email))
      return
    end
  end
`)
	}
	if verp {
		// VERP: rewrite the outbound envelope return-path so async DSNs come back
		// to the bounce domain carrying this message id. Applied here (reception)
		// because the message is mutated and persisted at this point.
		b.WriteString(`  do
    local mid = msg:id()
    if mid and tostring(mid) ~= '' then
      mid = tostring(mid)
      local mac = kumo.digest.hmac_sha256({ key_data = BOUNCE_VERP_SECRET }, mid)
      msg:set_sender(string.format('b+%s.%s@%s', string.sub(tostring(mac), 1, 16), mid, iris_bounce_domain(msg)))
    end
  end
`)
	}
	if rspamd {
		b.WriteString("  iris_rspamd_scan(msg, RSPAMD_ENFORCE)\n")
	}
	b.WriteString(`  local recipient = msg:recipient().email
  if is_suppressed(recipient) then
    iris_log_suppressed(msg, recipient)
    kumo.reject(550, '5.7.1 recipient is suppressed')
    return
  end
  iris_ensure_message_id(msg)
  iris_ensure_date(msg)
  iris_dkim_sign(msg)
  local class = classify_mail(msg)
  if not class then
    -- Fallback: classify by the connecting client's IP when no header did.
    class = classify_by_sender_ip(msg)
  end
  if class then
    msg:set_meta('mailclass', class)
  end
  local pool = select_pool(msg, recipient, class)
  if pool then
    msg:set_meta('tenant', pool)
  end
end)

`)
	return n
}

// senderIPClassifyLua defines the runtime helpers for sender-IP classification:
// pure-Lua IPv4/CIDR matching (no bitwise ops, for Lua-version safety) and the
// classify_by_sender_ip fallback. Non-IPv4 specs fall back to an exact match.
const senderIPClassifyLua = `
local function _ipv4_to_int(ip)
  local a, b, c, d = ip:match('^(%d+)%.(%d+)%.(%d+)%.(%d+)$')
  if not a then return nil end
  a, b, c, d = tonumber(a), tonumber(b), tonumber(c), tonumber(d)
  if a > 255 or b > 255 or c > 255 or d > 255 then return nil end
  return ((a * 256 + b) * 256 + c) * 256 + d
end

local function _ip_matches(ip, spec)
  local net_s, bits_s = spec:match('^(.-)/(%d+)$')
  if not net_s then return ip == spec end
  local ipi = _ipv4_to_int(ip)
  local neti = _ipv4_to_int(net_s)
  if not ipi or not neti then return false end
  local bits = tonumber(bits_s)
  if bits <= 0 then return true end
  if bits >= 32 then return ipi == neti end
  local div = 2 ^ (32 - bits)
  return math.floor(ipi / div) == math.floor(neti / div)
end

-- classify_by_sender_ip returns the mailclass configured for the connecting
-- client IP, or nil. Only consulted as a fallback when no header classified.
local function classify_by_sender_ip(msg)
  if #SENDER_IP_CLASSES == 0 then return nil end
  local ok, peer = pcall(function() return msg:get_meta('received_from') end)
  if not ok or peer == nil or peer == '' then return nil end
  peer = tostring(peer)
  local ip = peer:match('^(%d+%.%d+%.%d+%.%d+)') or peer
  for _, rule in ipairs(SENDER_IP_CLASSES) do
    if _ip_matches(ip, rule.cidr) then return rule.mailclass end
  end
  return nil
end
`

// writeSenderIPClasses emits the SENDER_IP_CLASSES table (CIDR → mailclass, in
// descending priority) plus the classification helpers. Rules are emitted in
// the same priority order as ROUTES so the highest-priority IP rule wins.
func writeSenderIPClasses(b *strings.Builder, routes []*RoutingRule) {
	b.WriteString("\n-- ===== sender-IP classification (fallback when no mailclass header) =====\n")
	b.WriteString("local SENDER_IP_CLASSES = {\n")
	for _, r := range sortedRoutes(routes) {
		if r.Status != RoutingStatusActive || r.MatchType != MatchSenderIP {
			continue
		}
		if r.MatchValue == "" || r.AssignMailclass == "" {
			continue
		}
		fmt.Fprintf(b, "  { cidr = %s, mailclass = %s },\n",
			MustLuaString(r.MatchValue), MustLuaString(r.AssignMailclass))
	}
	b.WriteString("}\n")
	b.WriteString(senderIPClassifyLua)
}

func writeQueueConfig(b *strings.Builder, snap ConfigSnapshot) {
	b.WriteString("-- ===== queue → egress pool mapping =====\n")
	b.WriteString("kumo.on('get_queue_config', function(domain, tenant, campaign, routing_domain)\n")
	if snap.LogStreamRedisURL != "" {
		// The log-stream tracker queue routes to the Redis-XADD custom_lua
		// constructor with its own retry policy so a Redis hiccup never blocks
		// real mail.
		b.WriteString(`  if domain == LOGSTREAM_TRACKER then
    return kumo.make_queue_config {
      protocol = { custom_lua = { constructor = 'make.redis_tracker' } },
      retry_interval = '30s',
      max_retry_interval = '5m',
    }
  end
`)
	}
	if bounceEnabled(snap) {
		// Inbound DSN routing: the reception hook tags bounce-domain mail with
		// queue=DSN_TRACKER; route it to the make.dsn_xadd custom_lua queue.
		b.WriteString(`  if domain == DSN_TRACKER then
    return kumo.make_queue_config {
      protocol = { custom_lua = { constructor = 'make.dsn_xadd' } },
      retry_interval = '30s',
      max_retry_interval = '5m',
    }
  end
`)
	}
	if dmarcEnabled(snap) {
		// Inbound DMARC routing: the reception hook tags report mail with
		// queue=DMARC_TRACKER; route it to the make.dmarc_xadd custom_lua queue.
		b.WriteString(`  if domain == DMARC_TRACKER then
    return kumo.make_queue_config {
      protocol = { custom_lua = { constructor = 'make.dmarc_xadd' } },
      retry_interval = '1m',
      max_retry_interval = '30m',
    }
  end
`)
	}
	if fblForwardEnabled(snap) {
		// FBL forward carrier sink: the original message is consumed here after its
		// content was re-injected to the forward address.
		b.WriteString(`  if domain == FBL_FORWARD_SINK then
    return kumo.make_queue_config {
      protocol = { custom_lua = { constructor = 'make.fbl_sink' } },
    }
  end
`)
	}
	if webhookEnabled(snap) {
		// Inbound webhook routing: the reception hook tags matched mail with
		// queue=WEBHOOK_TRACKER; route it to the make.webhook_post custom_lua queue.
		b.WriteString(`  if domain == WEBHOOK_TRACKER then
    return kumo.make_queue_config {
      protocol = { custom_lua = { constructor = 'make.webhook_post' } },
      retry_interval = '1m',
      max_retry_interval = '30m',
    }
  end
`)
	}
	if maildirEnabled(snap) {
		// Inbound maildir routing: the reception hook tags matched mail with the
		// synthetic maildir queue; kumod writes the message to the resolved Maildir.
		b.WriteString(`  if MAILDIR_PATHS[domain] then
    return kumo.make_queue_config {
      protocol = {
        maildir_path = MAILDIR_PATHS[domain],
        dir_mode = tonumber('700', 8),
        file_mode = tonumber('600', 8),
      },
      retry_interval = '1m',
      max_retry_interval = '20m',
    }
  end
`)
	}
	if forwardEnabled(snap) {
		// Inbound forward routing: the reception hook tags matched mail with the
		// synthetic forward queue; relay it to the pinned smarthost (bypassing MX).
		b.WriteString(`  do
    local fwd = FORWARD_SMARTHOSTS[domain]
    if fwd then
      return kumo.make_queue_config {
        protocol = { smtp = { mx_list = { fwd.mx } } },
        retry_interval = '5m',
        max_retry_interval = '30m',
      }
    end
  end
`)
	}
	// Default queue: egress pool + the configured delivery-rate retry schedule.
	b.WriteString(`  local pool = tenant
  if not pool or pool == '' then
    pool = 'default'
  end
  return kumo.make_queue_config {
    egress_pool = pool,
`)
	if v := strings.TrimSpace(snap.EgressRetryInterval); v != "" {
		fmt.Fprintf(b, "    retry_interval = %s,\n", MustLuaString(v))
	}
	if v := strings.TrimSpace(snap.EgressMaxRetryInterval); v != "" {
		fmt.Fprintf(b, "    max_retry_interval = %s,\n", MustLuaString(v))
	}
	if v := strings.TrimSpace(snap.EgressMaxAge); v != "" {
		fmt.Fprintf(b, "    max_age = %s,\n", MustLuaString(v))
	}
	b.WriteString(`  }
end)

`)
}

// bounceEnabled reports whether the DSN/bounce-domain pipeline is rendered.
// It requires both a bounce domain and a Redis stream to XADD DSNs into.
func bounceEnabled(snap ConfigSnapshot) bool {
	return strings.TrimSpace(snap.BounceDomain) != "" && snap.LogStreamRedisURL != ""
}

func writeDKIMSigning(b *strings.Builder) {
	// KumoMTA signs on reception, not on send: there is no
	// smtp_client_message_sending event. iris_dkim_sign is called from the
	// smtp_server_message_received reception hook (SMTP) and from
	// http_message_generated (HTTP injection), matching KumoMTA's reference
	// policy. The signature is applied to the received message and persists
	// through delivery.
	b.WriteString(`-- ===== message-id =====
-- RFC 5322 requires a Message-ID. Injecting apps (and bare SMTP clients) often
-- omit it, which trips rspamd's MISSING_MID and weakens threading. Add one when
-- absent, keyed to the From domain so it both aligns with and is covered by the
-- DKIM signature applied immediately after.
local function iris_ensure_message_id(msg)
  if msg:get_first_named_header_value('Message-ID') then return end
  local from = msg:from_header()
  local sender = msg:sender()
  local domain = (from and from.domain) or (sender and sender.domain) or 'localhost'
  msg:prepend_header('Message-ID', string.format('<%s@%s>', tostring(msg:id()), domain))
end

-- ===== date =====
-- RFC 5322 requires a Date. Injecting apps (and bare SMTP clients) sometimes
-- omit it, which trips rspamd's MISSING_DATE and -- because the DKIM signer
-- lists Date in its signed header set -- causes the signature to break when a
-- downstream MTA (e.g. a receiving Postfix) inserts the missing Date. Add one
-- when absent, before signing, so the Date is stable and covered by the DKIM
-- signature applied immediately after. UTC, RFC 5322 format.
local function iris_ensure_date(msg)
  if msg:get_first_named_header_value('Date') then return end
  msg:prepend_header('Date', os.date('!%a, %d %b %Y %H:%M:%S +0000'))
end

-- ===== dkim signing =====
-- Resolve a From domain to a signer: exact match first, then walk up the parent
-- labels so a key published at the organizational domain (example.com) also signs
-- its subdomains (infra.example.com). The matched domain becomes d=, which DMARC
-- relaxed-aligns with the From subdomain. The most specific configured domain
-- wins.
local function iris_dkim_lookup(from_domain)
  local d = string.lower(from_domain)
  while d and d ~= '' do
    local cfg = DKIM_BY_DOMAIN[d]
    if cfg then return d, cfg end
    d = d:match('%.(.+)$')
  end
  return nil, nil
end

local function iris_dkim_sign(msg)
  local from = msg:from_header()
  local domain = from and from.domain or nil
  if not domain then return end
  local sign_domain, cfg = iris_dkim_lookup(domain)
  if not cfg then return end
  local params = {
    domain = sign_domain,
    selector = cfg.selector,
    -- KumoMTA's rsa_sha256_signer requires an explicit header list; this is
    -- KumoMTA's recommended default set, plus Message-ID (ensured above).
    headers = { 'From', 'To', 'Subject', 'Date', 'Message-ID', 'MIME-Version', 'Content-Type', 'Sender' },
    key = cfg.key,
  }
  local signer
  if cfg.algo == 'ed25519' then
    signer = kumo.dkim.ed25519_signer(params)
  else
    signer = kumo.dkim.rsa_sha256_signer(params)
  end
  msg:dkim_sign(signer)
end

-- Sign mail injected via the HTTP API (the reception hook covers SMTP).
kumo.on('http_message_generated', function(msg)
  iris_ensure_message_id(msg)
  iris_ensure_date(msg)
  iris_dkim_sign(msg)
end)

`)
}

// writeLogStreamConsts emits the Redis log-stream constants (or a disabled
// marker). Setting LogStreamRedisURL is the trigger that turns logging on.
func writeLogStreamConsts(b *strings.Builder, snap ConfigSnapshot) {
	if snap.LogStreamRedisURL == "" {
		b.WriteString("-- ===== log stream (redis): disabled =====\n\n")
		return
	}
	name := snap.LogStreamName
	if name == "" {
		name = "iris.mail.events"
	}
	b.WriteString("-- ===== log stream (redis) =====\n")
	fmt.Fprintf(b, "local LOGSTREAM_REDIS_URL = %s\n", MustLuaString(snap.LogStreamRedisURL))
	fmt.Fprintf(b, "local LOGSTREAM_NAME      = %s\n", MustLuaString(name))
	b.WriteString("local LOGSTREAM_TRACKER   = \"iris_logger\"\n")
	b.WriteString("local LOGSTREAM_MAXLEN    = \"100000\"\n\n")
}

// writeInit emits the single kumo.on('init') handler: default listeners so the
// rendered policy is self-contained, plus configure_log_hook when the log
// stream is enabled (KumoMTA permits only one init handler).
func writeInit(b *strings.Builder, snap ConfigSnapshot) {
	httpListen := snap.HTTPListen
	if httpListen == "" {
		httpListen = "0.0.0.0:8000"
	}
	b.WriteString("-- ===== init =====\n")
	b.WriteString("kumo.on('init', function()\n")

	// One start_esmtp_listener per active listener. If none are configured,
	// fall back to a default so the policy still binds and receives.
	active := 0
	for _, l := range sortedListeners(snap.Listeners) {
		if l.Status != ListenerStatusActive {
			continue
		}
		writeEsmtpListener(b, l)
		active++
	}
	if active == 0 {
		esmtp := snap.EsmtpListen
		if esmtp == "" {
			esmtp = "0.0.0.0:2525"
		}
		fmt.Fprintf(b, "  kumo.start_esmtp_listener { listen = %s }\n", MustLuaString(esmtp))
	}
	fmt.Fprintf(b, "  kumo.start_http_listener { listen = %s }\n", MustLuaString(httpListen))

	// Spools are mandatory: kumod refuses to start ("No spools have been
	// defined") without them. Use the standard KumoMTA layout. configure_local_logs
	// records every log record (including Rejections, which are excluded from the
	// Redis hook below) to disk for operator inspection.
	b.WriteString("  kumo.define_spool { name = 'data', path = '/var/spool/kumomta/data' }\n")
	b.WriteString("  kumo.define_spool { name = 'meta', path = '/var/spool/kumomta/meta' }\n")
	// max_segment_duration forces time-based rotation. Without it KumoMTA only
	// rotates the (gzip-compressed) segment when it reaches max_file_size, so on a
	// low-volume relay the current segment stays open and buffered for hours and
	// recent records are not yet readable on disk (zcat/grep find nothing). One
	// minute keeps on-disk logs current for operator inspection.
	b.WriteString("  kumo.configure_local_logs { log_dir = '/var/log/kumomta', max_segment_duration = '1 minute' }\n")
	if f := strings.TrimSpace(snap.BounceClassifierFile); f != "" {
		// Load KumoMTA's bounce-classifier rules so Bounce log records carry a
		// classification (InvalidRecipient, SpamBlock, QuotaIssue, …).
		fmt.Fprintf(b, "  kumo.configure_bounce_classifier { files = { %s } }\n", MustLuaString(f))
	}
	if snap.LogStreamRedisURL != "" {
		// Stream the structured log records the Logs UI needs. The header
		// allow-list is dynamic: Subject plus every distinct header that a
		// mailclass routing rule matches on, so the Logs UI can show which
		// mailclass header/value applied. Rejection records are excluded from
		// the hook (they have no sender and spam the rfc5321 parser);
		// configure_local_logs still records them to disk.
		fmt.Fprintf(b, `  kumo.configure_log_hook {
    name = LOGSTREAM_TRACKER,
    headers = { %s },
    meta = { 'tenant', 'mailclass' },
    per_record = { Rejection = { enable = false } },
  }
`, luaLogHeaderList(snap.Routes))
	}
	b.WriteString("end)\n\n")
}

// loopbackRelayHost is always added to every listener's relay allowlist so
// on-box processes (local injection / submission) can always relay, independent
// of the operator-configured list.
const loopbackRelayHost = "127.0.0.1/32"

// writeEsmtpListener emits one kumo.start_esmtp_listener block for a listener.
func writeEsmtpListener(b *strings.Builder, l *Listener) {
	b.WriteString("  kumo.start_esmtp_listener {\n")
	fmt.Fprintf(b, "    listen = %s,\n", MustLuaString(l.ListenAddr()))
	fmt.Fprintf(b, "    hostname = %s,\n", MustLuaString(l.Hostname))
	if l.TLSEnabled {
		fmt.Fprintf(b, "    tls_certificate = %s,\n", MustLuaString(l.TLSCertPath))
		fmt.Fprintf(b, "    tls_private_key = %s,\n", MustLuaString(l.TLSKeyPath))
	}
	if l.MaxMessageSize > 0 {
		fmt.Fprintf(b, "    max_message_size = %d,\n", l.MaxMessageSize)
	}
	// The relay allowlist is authoritative (3.0.0): no RFC-1918 fallback. Loopback
	// (loopbackRelayHost) is ALWAYS permitted on every listener (:25 and :587
	// alike) so on-box local injection / submission works regardless of config;
	// everything else must be listed explicitly. An empty configured list
	// therefore renders loopback-only — the listener relays only for localhost and
	// otherwise accepts mail only for local/hosted domains (inbound-only / MX).
	parts := []string{MustLuaString(loopbackRelayHost)}
	seen := map[string]bool{loopbackRelayHost: true}
	for _, h := range l.RelayHosts {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		parts = append(parts, MustLuaString(h))
	}
	fmt.Fprintf(b, "    relay_hosts = { %s },\n", strings.Join(parts, ", "))
	b.WriteString("  }\n")
}

// writeEgressPaths emits per-VMTA connection limits and the require-TLS domain
// table. SOURCE_LIMITS is keyed by egress source (VMTA) name; REQUIRE_TLS_DOMAINS
// is keyed by destination domain. get_egress_path_config applies both: the
// connection limit for the source and enable_tls for a required-TLS domain, so
// kumod refuses to deliver to that domain in cleartext. Domains WITHOUT a
// require-TLS policy fall back to OpportunisticInsecure (encrypt if offered, do
// not hard-fail on cert verification) so legacy/retired receiver chains deliver
// instead of deferring.
// defaultPolicyDir is the directory the shaping sidecar files are loaded from
// when no explicit ShapingDir is configured (the standard KumoMTA policy dir,
// matching the example config_path). The apply adapter writes the files next to
// the policy, so the two always agree in a real deployment.
const defaultPolicyDir = "/opt/kumomta/etc/policy"

func writeEgressPaths(b *strings.Builder, vmtas []*VMTA, tlsPolicies []*TLSPolicy, fwdTargets []forwardTarget, shapingDir, tsaURL string) {
	b.WriteString("-- ===== egress path config (shaping helper: blueprints + warmup overrides) =====\n")
	b.WriteString("local SOURCE_LIMITS = {}\n")
	for _, v := range sortedVMTAs(vmtas) {
		if v.Status != VMTAStatusActive && v.Status != VMTAStatusDraining {
			continue
		}
		if v.MaxConnections > 0 {
			fmt.Fprintf(b, "SOURCE_LIMITS[%s] = %d\n", MustLuaString(v.Name), v.MaxConnections)
		}
	}
	b.WriteString("local REQUIRE_TLS_DOMAINS = {}\n")
	for _, p := range sortedTLSPolicies(tlsPolicies) {
		if p.Status != TLSPolicyActive {
			continue
		}
		fmt.Fprintf(b, "REQUIRE_TLS_DOMAINS[%s] = %s\n",
			MustLuaString(strings.ToLower(p.Domain)), MustLuaString(p.EnableTLSValue()))
	}
	// Forward routes deliver to a synthetic queue whose domain is the smarthost
	// key; honor their TLS policy on that egress path. Opportunistic is kumod's
	// default and needs no entry.
	for _, t := range fwdTargets {
		switch t.tls {
		case ForwardTLSRequired:
			fmt.Fprintf(b, "REQUIRE_TLS_DOMAINS[%s] = %s\n", MustLuaString(t.key), MustLuaString("Required"))
		case ForwardTLSNone:
			fmt.Fprintf(b, "REQUIRE_TLS_DOMAINS[%s] = %s\n", MustLuaString(t.key), MustLuaString("Disabled"))
		}
	}

	// Delivery limits (blueprints base + per-IP warmup overrides) come from
	// KumoMTA's shaping helper, which natively handles provider grouping. iris
	// overlays its generous timeouts (KumoMTA's ~5s per-command defaults are too
	// aggressive for slow/tarpitting receivers), the per-source connection cap
	// (VMTA MaxConnections), and require-TLS. Verified against a live kumod.
	dir := strings.TrimRight(strings.TrimSpace(shapingDir), "/")
	if dir == "" {
		dir = defaultPolicyDir
	}
	// The shaping helper (setup_with_automation) loads ONLY iris's files
	// (no_default_files: blueprints are authoritative, no community-shaping leak)
	// and, when a TSA daemon URL is configured, publishes log events to it and
	// subscribes for adaptive (hourly) back-off layered under the warmup ceiling.
	// skip_make=true returns a mergeable params table so iris overlays timeouts /
	// connection cap / require-TLS. Coexistence with iris's log + queue hooks and
	// the no-leak resolution were verified against a live kumod.
	b.WriteString("\nlocal shaping = require 'policy-extras.shaping'\n")
	fmt.Fprintf(b, "local iris_shaper = shaping:setup_with_automation {\n  no_default_files = true,\n  extra_files = { %s, %s },\n",
		MustLuaString(dir+"/iris-base.toml"), MustLuaString(dir+"/iris-warmup.toml"))
	if tsa := strings.TrimSpace(tsaURL); tsa != "" {
		fmt.Fprintf(b, "  publish = { %s },\n  subscribe = { %s },\n", MustLuaString(tsa), MustLuaString(tsa))
	}
	b.WriteString("}\n")
	b.WriteString(`
kumo.on('get_egress_path_config', function(domain, egress_source, site_name)
  local params = iris_shaper.get_egress_path_config(domain, egress_source, site_name, true)
  params.connect_timeout = params.connect_timeout or '30s'
  params.ehlo_timeout = params.ehlo_timeout or '30s'
  params.mail_from_timeout = params.mail_from_timeout or '30s'
  params.rcpt_to_timeout = params.rcpt_to_timeout or '30s'
  params.rset_timeout = params.rset_timeout or '30s'
  params.starttls_timeout = params.starttls_timeout or '30s'
  params.data_timeout = params.data_timeout or '60s'
  params.data_dot_timeout = params.data_dot_timeout or '120s'
  params.idle_timeout = params.idle_timeout or '60s'
  local limit = SOURCE_LIMITS[egress_source]
  if limit and limit > 0 and (not params.connection_limit or params.connection_limit > limit) then
    params.connection_limit = limit
  end
  local tls = REQUIRE_TLS_DOMAINS[string.lower(domain)]
  if tls then
    params.enable_tls = tls
  else
    -- Baseline for port-25 delivery: encrypt opportunistically but do NOT hard-
    -- fail on certificate verification. Many large receivers (e.g. Outlook) still
    -- chain through roots the current trust store has retired, or serve legacy/
    -- incomplete chains; kumod's verifying default turns those into deferrals that
    -- silently drag deliverability. Require-TLS domains above keep real
    -- verification (Required). Matches Postfix's default posture for :25.
    params.enable_tls = params.enable_tls or 'OpportunisticInsecure'
  end
  return kumo.make_egress_path(params)
end)

`)
}

func sortedTLSPolicies(in []*TLSPolicy) []*TLSPolicy {
	out := append([]*TLSPolicy(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out
}

func sortedListeners(in []*Listener) []*Listener {
	out := append([]*Listener(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// luaLogHeaderList builds the configure_log_hook header allow-list: always
// "From" and "Subject", plus each distinct header name matched by an active
// mailclass routing rule, rendered as escaped Lua string literals. "From" is
// included so log records carry the original From header — the envelope sender
// is VERP-rewritten at reception, so it is the only place the original sender
// survives.
func luaLogHeaderList(routes []*RoutingRule) string {
	seen := map[string]struct{}{}
	headers := []string{"From", "Subject"}
	seen["from"] = struct{}{}
	seen["subject"] = struct{}{}
	var mailclass []string
	for _, r := range routes {
		if r.Status != RoutingStatusActive || r.MatchType != MatchMailclass {
			continue
		}
		h := r.MatchHeader
		if h == "" {
			h = DefaultMailClassHeader
		}
		key := strings.ToLower(h)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		mailclass = append(mailclass, h)
	}
	sort.Strings(mailclass)
	headers = append(headers, mailclass...)

	parts := make([]string, len(headers))
	for i, h := range headers {
		parts[i] = MustLuaString(h)
	}
	return strings.Join(parts, ", ")
}

// DSNStreamName is the Redis stream the DSN catcher XADDs inbound bounces onto;
// the Iris DSN consumer reads it.
const DSNStreamName = "iris.dsn.events"

// fblEnabled reports whether the FBL feedback pipeline is rendered (any approved
// ARF domain or any awaiting-approval forward). The log_arf parsing works on its
// own; the resulting Feedback record only reaches iris when the log hook
// (LogStreamRedisURL) is also configured.
func fblEnabled(snap ConfigSnapshot) bool {
	return len(snap.FBLEndpoints) > 0
}

// fblApprovedDomains returns the deduped set of domains with an approved FBL
// endpoint (these enable log_arf ARF parsing).
func fblApprovedDomains(snap ConfigSnapshot) map[string]bool {
	out := map[string]bool{}
	for _, e := range snap.FBLEndpoints {
		if e == nil || e.Status != FBLApproved {
			continue
		}
		if d := SanitizeAddress(e.Domain); d != "" {
			out[d] = true
		}
	}
	return out
}

// fblForwards returns the awaiting-approval endpoints whose mail should be
// forwarded. Endpoints on a domain that is also approved are excluded: log_arf
// is per-domain, so an approved entry forces ARF parsing for the whole domain
// and the forward could never fire (approved wins).
func fblForwards(snap ConfigSnapshot) []*FBLEndpoint {
	approved := fblApprovedDomains(snap)
	var out []*FBLEndpoint
	for _, e := range snap.FBLEndpoints {
		if e == nil || e.Status != FBLAwaitingApproval {
			continue
		}
		d := strings.ToLower(strings.TrimSpace(e.Domain))
		addr := strings.ToLower(strings.TrimSpace(e.FeedbackAddress))
		fwd := strings.ToLower(strings.TrimSpace(e.ForwardAddress))
		if d == "" || addr == "" || fwd == "" || approved[d] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// fblForwardPool returns the egress pool (first active VMTA's singleton pool)
// used to deliver forwarded feedback mail, so it egresses from a real source IP
// instead of the address-less default pool. Empty when there is no active VMTA.
func fblForwardPool(snap ConfigSnapshot) string {
	for _, v := range sortedVMTAs(snap.VMTAs) {
		if v.Status == VMTAStatusActive {
			return v.Name
		}
	}
	return ""
}

// fblForwardEnabled reports whether any awaiting-approval forward is rendered
// (and therefore whether the reception-hook forward block is emitted).
func fblForwardEnabled(snap ConfigSnapshot) bool {
	return len(fblForwards(snap)) > 0
}

// BounceDomainPlaceholder is the token in BounceDomainTemplate that is replaced
// with each sending domain to derive that domain's aligned bounce domain.
const BounceDomainPlaceholder = "{domain}"

// bounceDomainsByFrom builds the per-From-domain bounce-domain map by applying
// BounceDomainTemplate to every configured DKIM (sending) domain. Returns an
// empty map when the template is unset or lacks the {domain} placeholder, so the
// global BounceDomain remains in effect. Keyed and valued in lower case.
func bounceDomainsByFrom(snap ConfigSnapshot) map[string]string {
	tmpl := strings.ToLower(strings.TrimSpace(snap.BounceDomainTemplate))
	if tmpl == "" || !strings.Contains(tmpl, BounceDomainPlaceholder) {
		return nil
	}
	out := make(map[string]string, len(snap.DKIM))
	for _, d := range snap.DKIM {
		from := strings.ToLower(strings.TrimSpace(d.Domain))
		if from == "" {
			continue
		}
		out[from] = strings.ReplaceAll(tmpl, BounceDomainPlaceholder, from)
	}
	return out
}

// writeBounceConsts emits the bounce/DSN + FBL constants (empty when disabled).
// Three FBL tables are rendered: FBL_DOMAINS (approved domains → ARF parsing),
// FBL_FORWARD (feedback address → forward address, for awaiting approval), and
// FBL_RELAY_DOMAINS (awaiting-approval domains the listener must relay).
func writeBounceConsts(b *strings.Builder, snap ConfigSnapshot) {
	b.WriteString("-- ===== bounce / DSN + FBL pipeline constants =====\n")
	bounceDomain, dsnTracker, dsnStream := "", "", ""
	if bounceEnabled(snap) {
		bounceDomain = SanitizeAddress(snap.BounceDomain)
		dsnTracker = "iris_dsn_catcher"
		dsnStream = DSNStreamName
	}
	fmt.Fprintf(b, "local BOUNCE_DOMAIN = %s\n", MustLuaString(bounceDomain))
	fmt.Fprintf(b, "local DSN_TRACKER   = %s\n", MustLuaString(dsnTracker))
	fmt.Fprintf(b, "local DSN_STREAM    = %s\n", MustLuaString(dsnStream))
	// Per-sending-domain bounce domains derived from BounceDomainTemplate.
	// BOUNCE_DOMAIN_BY_FROM maps a From-domain to its bounce domain (outbound
	// VERP selection); BOUNCE_DOMAINS is the reverse set of those bounce domains
	// (inbound DSN acceptance). Both are empty unless the template is configured.
	b.WriteString("local BOUNCE_DOMAIN_BY_FROM = {}\n")
	b.WriteString("local BOUNCE_DOMAINS = {}\n")
	byFrom := bounceDomainsByFrom(snap)
	fromDomains := make([]string, 0, len(byFrom))
	for from := range byFrom {
		fromDomains = append(fromDomains, from)
	}
	sort.Strings(fromDomains)
	for _, from := range fromDomains {
		bd := byFrom[from]
		fmt.Fprintf(b, "BOUNCE_DOMAIN_BY_FROM[%s] = %s\n", MustLuaString(from), MustLuaString(bd))
		fmt.Fprintf(b, "BOUNCE_DOMAINS[%s] = true\n", MustLuaString(bd))
	}
	if verpEnabled(snap) {
		// iris_bounce_domain returns the From-domain's bounce domain (for VERP),
		// falling back to the global BOUNCE_DOMAIN. Emitted only when VERP rewrites
		// the envelope (its sole caller), so an unused local is never rendered.
		b.WriteString(`local function iris_bounce_domain(msg)
  local fh = msg:from_header()
  local fdom = fh and fh.domain and string.lower(fh.domain) or nil
  if fdom and BOUNCE_DOMAIN_BY_FROM[fdom] then
    return BOUNCE_DOMAIN_BY_FROM[fdom]
  end
  return BOUNCE_DOMAIN
end
`)
	}
	b.WriteString("local FBL_DOMAINS  = {}\n")
	b.WriteString("local FBL_FORWARD  = {}\n")
	b.WriteString("local FBL_RELAY_DOMAINS = {}\n")
	approved := fblApprovedDomains(snap)
	approvedSorted := make([]string, 0, len(approved))
	for d := range approved {
		approvedSorted = append(approvedSorted, d)
	}
	sort.Strings(approvedSorted)
	for _, d := range approvedSorted {
		fmt.Fprintf(b, "FBL_DOMAINS[%s] = true\n", MustLuaString(d))
	}
	for _, e := range fblForwards(snap) {
		addr := SanitizeAddress(e.FeedbackAddress)
		fwd := SanitizeAddress(e.ForwardAddress)
		dom := SanitizeAddress(e.Domain)
		fmt.Fprintf(b, "FBL_FORWARD[%s] = %s\n", MustLuaString(addr), MustLuaString(fwd))
		fmt.Fprintf(b, "FBL_RELAY_DOMAINS[%s] = true\n", MustLuaString(dom))
	}
	// Egress pool forwarded feedback mail is sent through (a real VMTA source, so
	// it leaves from a known IP rather than the address-less default pool). Empty
	// when no active VMTA exists.
	fmt.Fprintf(b, "local FBL_FORWARD_POOL = %s\n", MustLuaString(fblForwardPool(snap)))
	// DMARC aggregate-report catcher constants (empty when disabled).
	dmarcAddr, dmarcDomain, dmarcStream, dmarcTracker := "", "", "", ""
	if dmarcEnabled(snap) {
		dmarcAddr = SanitizeAddress(snap.DMARCReportAddr)
		dmarcDomain = RecipientDomain(dmarcAddr)
		dmarcStream = DMARCStreamName
		dmarcTracker = "iris_dmarc_catcher"
	}
	b.WriteString("local FBL_FORWARD_SINK = \"iris_fbl_sink\"\n")
	fmt.Fprintf(b, "local DMARC_REPORT_ADDR   = %s\n", MustLuaString(dmarcAddr))
	fmt.Fprintf(b, "local DMARC_REPORT_DOMAIN = %s\n", MustLuaString(dmarcDomain))
	fmt.Fprintf(b, "local DMARC_STREAM        = %s\n", MustLuaString(dmarcStream))
	fmt.Fprintf(b, "local DMARC_TRACKER       = %s\n", MustLuaString(dmarcTracker))
	b.WriteString("\n")
}

// DMARCStreamName is the Redis stream the DMARC catcher XADDs inbound aggregate
// reports onto; the Iris DMARC consumer reads it.
const DMARCStreamName = "iris.dmarc.events"

// dmarcEnabled reports whether the DMARC aggregate-report capture pipeline is
// rendered. It needs the report address plus the log-stream Redis (the catcher
// XADDs onto Redis).
func dmarcEnabled(snap ConfigSnapshot) bool {
	return strings.TrimSpace(snap.DMARCReportAddr) != "" && snap.LogStreamRedisURL != ""
}

// writeListenerDomain emits the single get_listener_domain handler: it relays
// the bounce domain into the chain (DSN catcher), enables ARF parsing (log_arf)
// for approved FBL domains, relays awaiting-approval FBL domains (the reception
// hook forwards their feedback mail), and relays inbound mail for any webhook
// domain. Emitted when any of those pipelines is configured (the event may be
// defined once). The approved (ARF) check precedes the awaiting (relay) check so
// an approved domain always parses ARF for the whole domain.
func writeListenerDomain(b *strings.Builder, snap ConfigSnapshot) {
	if !bounceEnabled(snap) && !fblEnabled(snap) && !inboundRoutesEnabled(snap) && !dmarcEnabled(snap) {
		return
	}
	b.WriteString(`-- Accept inbound mail for the bounce domain (relayed into the chain, where the
-- reception hook routes it to the DSN tracker), parse ARF reports at any approved
-- FBL domain (emitting Feedback log records), relay awaiting-approval FBL domains
-- (the reception hook forwards their feedback mail), and relay webhook domains
-- (routed to the webhook poster).
kumo.on('get_listener_domain', function(domain, listener)
  if (BOUNCE_DOMAIN ~= '' and domain == BOUNCE_DOMAIN) or BOUNCE_DOMAINS[domain] then
    return kumo.make_listener_domain { relay_to = true }
  end
  if FBL_DOMAINS[domain] then
    return kumo.make_listener_domain { log_arf = 'LogThenDrop' }
  end
  if FBL_RELAY_DOMAINS[domain] then
    return kumo.make_listener_domain { relay_to = true }
  end
  if DMARC_REPORT_DOMAIN ~= '' and domain == DMARC_REPORT_DOMAIN then
    return kumo.make_listener_domain { relay_to = true }
  end
`)
	if inboundRoutesEnabled(snap) {
		b.WriteString(`  if ROUTE_DOMAINS[domain] then
    return kumo.make_listener_domain { relay_to = true }
  end
`)
	}
	b.WriteString(`  return nil
end)

`)
}

// verpEnabled reports whether the VERP envelope rewrite is rendered (requires
// the bounce pipeline plus a signing secret).
func verpEnabled(snap ConfigSnapshot) bool {
	return bounceEnabled(snap) && strings.TrimSpace(snap.BounceVerpSecret) != ""
}

// writeBounceVerp emits the VERP signing secret as a local so the reception
// hook can rewrite the envelope return-path. The rewrite itself is applied in
// smtp_server_message_received (the message is mutated and persisted there;
// there is no client-sending hook), see writeRouting's verp block.
func writeBounceVerp(b *strings.Builder, snap ConfigSnapshot) {
	if !verpEnabled(snap) {
		return
	}
	fmt.Fprintf(b, "-- ===== bounce VERP (envelope return-path) =====\nlocal BOUNCE_VERP_SECRET = %s\n\n", MustLuaString(strings.TrimSpace(snap.BounceVerpSecret)))
}

// writeDsnCatcher emits the make.dsn_xadd custom_lua queue constructor that XADDs
// raw inbound bounce messages onto the DSN Redis stream. No-op when the bounce
// pipeline is disabled. (The get_listener_domain handler is emitted separately
// by writeListenerDomain so it can also serve the FBL domain.)
func writeDsnCatcher(b *strings.Builder, snap ConfigSnapshot) {
	if !bounceEnabled(snap) {
		return
	}
	b.WriteString(`kumo.on('make.dsn_xadd', function(_domain, _tenant, _campaign)
  local redis = require 'redis'
  local conn = redis.open { node = LOGSTREAM_REDIS_URL, pool_size = 5 }
  local connection = {}
  function connection:send(message)
    local rcpt = message:recipient()
    local payload = message:get_data()
    local ok, err = pcall(function()
      conn:query('XADD', DSN_STREAM, 'MAXLEN', '~', '50000', '*',
                 'recipient', (rcpt and rcpt.email) or '',
                 'data', payload)
    end)
    if not ok then
      kumo.log_error('dsn: xadd failed err=' .. tostring(err))
      return '250 dsn redis unavailable, dropped'
    end
    return '250 dsn captured'
  end
  return connection
end)

`)
}

// writeDMARCCatcher emits the make.dmarc_xadd custom_lua queue constructor that
// XADDs raw inbound DMARC aggregate reports onto the DMARC Redis stream. No-op
// when the DMARC pipeline is disabled. The report parser consumes the stream.
func writeDMARCCatcher(b *strings.Builder, snap ConfigSnapshot) {
	if !dmarcEnabled(snap) {
		return
	}
	b.WriteString(`kumo.on('make.dmarc_xadd', function(_domain, _tenant, _campaign)
  local redis = require 'redis'
  local conn = redis.open { node = LOGSTREAM_REDIS_URL, pool_size = 3 }
  local connection = {}
  function connection:send(message)
    local payload = message:get_data()
    local ok, err = pcall(function()
      conn:query('XADD', DMARC_STREAM, 'MAXLEN', '~', '50000', '*', 'data', payload)
    end)
    if not ok then
      kumo.log_error('dmarc: xadd failed err=' .. tostring(err))
      return '250 dmarc redis unavailable, dropped'
    end
    return '250 dmarc captured'
  end
  return connection
end)

`)
}

// writeFBLSink emits the make.fbl_sink custom_lua queue that consumes (drops)
// the original carrier message after its content has been re-injected to the
// forward address. No-op when no awaiting-approval forward is configured.
func writeFBLSink(b *strings.Builder, snap ConfigSnapshot) {
	if !fblForwardEnabled(snap) {
		return
	}
	b.WriteString(`kumo.on('make.fbl_sink', function(_domain, _tenant, _campaign)
  local connection = {}
  function connection:send(_message)
    return '250 fbl carrier consumed'
  end
  return connection
end)

`)
}

// writeLogHook emits the should_enqueue_log_record filter and the
// make.redis_tracker custom_lua queue constructor that XADDs each tracked log
// record onto the Redis stream. The XADD is fail-open: a Redis hiccup logs an
// error and acks 250 so real mail keeps flowing.
func writeLogHook(b *strings.Builder, snap ConfigSnapshot) {
	if snap.LogStreamRedisURL == "" {
		return
	}
	b.WriteString(`-- ===== log hook (redis stream) =====
kumo.on('should_enqueue_log_record', function(msg, hook_name)
  if hook_name ~= LOGSTREAM_TRACKER then return false end
  local lr = msg:get_meta 'log_record'
  if not lr then return false end
  if lr.queue == LOGSTREAM_TRACKER then return false end
  local tracked = {
    Reception = true, Delivery = true, Bounce = true,
    TransientFailure = true, Feedback = true,
    -- Terminal queue removals: admin bounce (operator purge) and age expiration.
    -- Without these, a purged/expired message keeps its last "deferred" status in
    -- the Logs/Queues UI even though kumod has dropped it.
    AdminBounce = true, Expiration = true,
  }
  if tracked[lr.type] then
    msg:set_meta('queue', LOGSTREAM_TRACKER)
    return true
  end
  return false
end)

kumo.on('make.redis_tracker', function(_domain, _tenant, _campaign)
  local redis = require 'redis'
  local conn = redis.open { node = LOGSTREAM_REDIS_URL, pool_size = 10 }
  local connection = {}
  function connection:send(message)
    local lr = message:get_meta 'log_record'
    local etype = (lr and lr.type) or 'Unknown'
    local payload = message:get_data()
    local ok, err = pcall(function()
      conn:query('XADD', LOGSTREAM_NAME, 'MAXLEN', '~', LOGSTREAM_MAXLEN, '*',
                 'type', etype, 'data', payload)
    end)
    if not ok then
      kumo.log_error('logstream: xadd failed type=' .. etype .. ' err=' .. tostring(err))
      return string.format('250 logstream redis unavailable, dropped %s', etype)
    end
    return string.format('250 streamed %s', etype)
  end
  return connection
end)

`)
}

// writeHostedDomains emits the HOSTED_DOMAINS set used to scope inbound rspamd
// scanning. Always declared so the rspamd function can index it unconditionally.
// Inbound-route domains are hosted by definition (we accept and locally process
// their mail), so they are unioned in — otherwise route-captured mail would be
// skipped by the HOSTED_DOMAINS guard in iris_rspamd_scan.
func writeHostedDomains(b *strings.Builder, snap ConfigSnapshot) {
	b.WriteString("-- ===== hosted (inbound) domains =====\n")
	b.WriteString("local HOSTED_DOMAINS = {}\n")
	seen := map[string]bool{}
	emit := func(d string) {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		fmt.Fprintf(b, "HOSTED_DOMAINS[%s] = true\n", MustLuaString(d))
	}
	for _, d := range snap.hostedDomains() {
		emit(d)
	}
	for _, r := range sortedActiveRoutes(snap) {
		emit(r.RouteDomain())
	}
	b.WriteString("\n")
}

// writeRspamd emits the iris_rspamd_scan function (or a no-op stub) that scans
// inbound-to-hosted mail through rspamd's /checkv2, adds X-Spam headers, and —
// in enforce mode — honors reject/greylist verdicts. Fail-open throughout.
func writeRspamd(b *strings.Builder, snap ConfigSnapshot) {
	// Emit the scanner whenever the machinery is available — the global mode is
	// tag/enforce, or any inbound route opts into scanning — so per-route scanning
	// works even when the deployment-wide mode is off.
	if !rspamdMachineryEnabled(snap) {
		b.WriteString("-- ===== inbound spam filtering (rspamd): disabled =====\n")
		b.WriteString("local function iris_rspamd_scan(_msg, _enforce) end\n\n")
		return
	}
	b.WriteString("-- ===== inbound spam filtering (rspamd) =====\n")
	fmt.Fprintf(b, "local RSPAMD_URL = %s\n", MustLuaString(strings.TrimSpace(snap.RspamdURL)))
	// Effective enforce flag for the global (non-route) inbound path.
	fmt.Fprintf(b, "local RSPAMD_ENFORCE = %t\n", snap.rspamdEnforce())
	// publishResults emits the verdict-recording block when a Redis stream is
	// configured; without it scanning still tags mail but the Rspamd Results page
	// has no producer. Stream name is shared with the ingestion worker via
	// RspamdResultsStream. The XADD is fail-open and runs before the enforce-mode
	// reject branches so rejected mail is still recorded.
	if snap.LogStreamRedisURL != "" {
		fmt.Fprintf(b, "local RSPAMD_RESULTS_STREAM = %s\n", MustLuaString(RspamdResultsStream))
	}
	b.WriteString(`local function iris_rspamd_scan(msg, enforce)
  local rcpt = msg:recipient()
  local rdom = (rcpt and rcpt.domain or ''):lower()
  -- Inbound-to-hosted only: never scan outbound relay or system mail.
  if not HOSTED_DOMAINS[rdom] then return end
  local s = msg:sender()
  local client = kumo.http.build_client {}
  local req = client:post(RSPAMD_URL .. '/checkv2')
  req:header('From', (s and s.email) or '')
  req:header('Rcpt', (rcpt and rcpt.email) or '')
  req:header('Deliver-To', (rcpt and rcpt.email) or '')
  req:body(msg:get_data())
  local ok, resp = pcall(function() return req:send() end)
  if not ok then
    kumo.log_error('rspamd: request failed: ' .. tostring(resp))
    return -- fail-open
  end
  if resp:status_code() ~= 200 then
    kumo.log_error('rspamd: unexpected status ' .. tostring(resp:status_code()))
    return -- fail-open
  end
  local ok2, r = pcall(function() return kumo.serde.json_parse(resp:text()) end)
  if not ok2 or type(r) ~= 'table' then return end -- fail-open
  local score = r.score or 0
  local action = r.action or 'no action'
  msg:prepend_header('X-Spam-Score', string.format('%.2f', score))
  msg:prepend_header('X-Rspamd-Action', tostring(action))
`)
	if snap.LogStreamRedisURL != "" {
		b.WriteString(`  -- Record the verdict on the Redis stream the rspamd ingestion worker drains.
  -- Symbols are flattened to a sorted name array; reason is rspamd's smtp_message
  -- when present. Fail-open: a Redis hiccup logs and never blocks the message.
  local symbols = {}
  if type(r.symbols) == 'table' then
    for name, _ in pairs(r.symbols) do symbols[#symbols + 1] = name end
    table.sort(symbols)
  end
  local reason = ''
  if type(r.messages) == 'table' and r.messages.smtp_message then
    reason = tostring(r.messages.smtp_message)
  end
  local okx, errx = pcall(function()
    local redis = require 'redis'
    local conn = redis.open { node = LOGSTREAM_REDIS_URL, pool_size = 5 }
    conn:query('XADD', RSPAMD_RESULTS_STREAM, 'MAXLEN', '~', '50000', '*',
               'message_id', tostring(msg:id()),
               'action', tostring(action),
               'score', string.format('%.4f', score),
               'symbols', kumo.serde.json_encode(symbols),
               'reason', reason)
  end)
  if not okx then
    kumo.log_error('rspamd: results xadd failed err=' .. tostring(errx))
  end
`)
	}
	b.WriteString(`  if action == 'reject' then
    if enforce then
      kumo.reject(550, '5.7.1 message rejected as spam')
      return
    end
    msg:prepend_header('X-Spam', 'yes')
  elseif action == 'soft reject' or action == 'greylist' then
    if enforce then
      kumo.reject(451, '4.7.1 greylisted, please try again later')
      return
    end
  elseif action == 'add header' or action == 'rewrite subject' then
    msg:prepend_header('X-Spam', 'yes')
    msg:prepend_header('X-Spam-Status', string.format('Yes, score=%.2f', score))
  end
end

`)
}

func sortedVMTAs(in []*VMTA) []*VMTA {
	out := append([]*VMTA(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func sortedGroups(in []*VMTAGroup) []*VMTAGroup {
	out := append([]*VMTAGroup(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sortedRoutes orders rules by descending priority (higher priority wins), with
// rule name as a stable tiebreaker for deterministic rendering.
func sortedRoutes(in []*RoutingRule) []*RoutingRule {
	out := append([]*RoutingRule(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Name < out[j].Name
	})
	return out
}
func sortedDKIM(in []*DKIMDomain) []*DKIMDomain {
	out := append([]*DKIMDomain(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out
}

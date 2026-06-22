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
	// InboundWebhooks are active webhook rules. Their recipient domains are
	// relay-accepted by get_listener_domain, and matching inbound mail is routed
	// to the in-policy webhook poster (make.webhook_post) which forwards the raw
	// message to the destination URL. Includes the HMAC secret (SecretRef).
	InboundWebhooks []*WebhookRule
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
	// log records (Reception/Delivery/Bounce/TransientFailure/Feedback) into a
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

	// BounceClassifierFile, when set, makes the init block load KumoMTA's bounce
	// classifier rules so Bounce log records carry a classification category.
	BounceClassifierFile string

	// FBLDomain, when set, makes kumod parse RFC 5965 ARF feedback reports sent
	// to this domain (log_arf) and emit a Feedback log record, which the log hook
	// streams to the feedback consumer (auto-suppression). Requires the log hook
	// (LogStreamRedisURL) to actually reach iris.
	FBLDomain string

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
	writeWebhooks(&b, snap)
	// Listener-domain handler (bounce relay + FBL ARF parsing + webhook relay)
	// and the DSN XADD constructor.
	writeListenerDomain(&b, snap)
	writeDsnCatcher(&b, snap)
	// Egress sources (one per active VMTA).
	rendered := writeEgressSources(&b, snap.VMTAs, snap.EgressEHLODefault)
	// Egress pools: a singleton pool per VMTA + one per active group.
	pools := writeEgressPools(&b, snap.VMTAs, snap.Groups, vmtaName)
	// Per-VMTA connection limits (max_connections) via the egress path config.
	writeEgressPaths(&b, snap.VMTAs)
	// DKIM signers.
	dkim := writeDKIMTable(&b, snap.DKIM)
	// DKIM signing function + the http-injection signing hook. Defined before the
	// reception hook so that hook can call iris_dkim_sign as an in-scope upvalue.
	writeDKIMSigning(&b)
	// Hosted domains + inbound rspamd scanning.
	writeHostedDomains(&b, snap)
	writeRspamd(&b, snap)
	// Suppression lookup tables.
	supp := writeSuppression(&b, snap.Suppressions)
	// Routing table (priority-ordered) and reception hook (which signs DKIM).
	routes := writeRouting(&b, snap.Routes, vmtaName, groupName, snap.rspamdEnabled(), bounceEnabled(snap), webhookEnabled(snap))
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
		SuppressionCount: supp,
		Valid:            len(issues) == 0,
		LintIssues:       issues,
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

func writeSuppression(b *strings.Builder, supps []*SuppressionEntry) int {
	b.WriteString("-- ===== suppression list =====\n")
	b.WriteString("local SUPPRESSED_EMAILS = {}\n")
	b.WriteString("local SUPPRESSED_DOMAINS = {}\n")
	n := 0
	for _, s := range sortedSuppressions(supps) {
		if s.Status != SuppressActive {
			continue
		}
		switch s.Type {
		case SuppressEmail:
			fmt.Fprintf(b, "SUPPRESSED_EMAILS[%s] = true\n", MustLuaString(s.Value))
			n++
		case SuppressDomain:
			fmt.Fprintf(b, "SUPPRESSED_DOMAINS[%s] = true\n", MustLuaString(s.Value))
			n++
		}
	}
	b.WriteString(`
local function is_suppressed(recipient)
  local domain = recipient:match('@(.+)$') or ''
  return SUPPRESSED_EMAILS[recipient] == true or SUPPRESSED_DOMAINS[domain] == true
end

`)
	return n
}

func writeRouting(b *strings.Builder, routes []*RoutingRule, vmtaName, groupName map[string]string, rspamd, bounce, webhook bool) int {
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
    if rdom == BOUNCE_DOMAIN then
      msg:set_meta('queue', DSN_TRACKER)
      return
    end
  end
`)
	}
	if webhook {
		// Inbound webhook capture: recipient-matched mail is routed to the webhook
		// poster queue (before suppression/classification) instead of being relayed
		// onward.
		b.WriteString(`  do
    local rcpt = msg:recipient()
    local email = (rcpt and rcpt.email or ''):lower()
    local rdom = (rcpt and rcpt.domain or ''):lower()
    if WEBHOOK_BY_EMAIL[email] or WEBHOOK_BY_DOMAIN[rdom] then
      msg:set_meta('queue', WEBHOOK_TRACKER)
      return
    end
  end
`)
	}
	if rspamd {
		b.WriteString("  iris_rspamd_scan(msg)\n")
	}
	b.WriteString(`  local recipient = msg:recipient().email
  if is_suppressed(recipient) then
    kumo.reject(550, '5.7.1 recipient is suppressed')
    return
  end
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
	b.WriteString(`-- ===== dkim signing =====
local function iris_dkim_sign(msg)
  local from = msg:from_header()
  local domain = from and from.domain or nil
  if not domain then return end
  local cfg = DKIM_BY_DOMAIN[string.lower(domain)]
  if not cfg then return end
  local params = {
    domain = string.lower(domain),
    selector = cfg.selector,
    -- KumoMTA's rsa_sha256_signer requires an explicit header list; this is
    -- KumoMTA's recommended default set.
    headers = { 'From', 'To', 'Subject', 'Date', 'MIME-Version', 'Content-Type', 'Sender' },
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
	b.WriteString("  kumo.configure_local_logs { log_dir = '/var/log/kumomta' }\n")
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

// defaultRelayHosts is the relay allowlist used when a listener configures none
// (RFC 1918 private ranges + loopback).
var defaultRelayHosts = []string{"127.0.0.1/32", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

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
	relay := l.RelayHosts
	if len(relay) == 0 {
		relay = defaultRelayHosts
	}
	parts := make([]string, 0, len(relay))
	for _, h := range relay {
		h = strings.TrimSpace(h)
		if h != "" {
			parts = append(parts, MustLuaString(h))
		}
	}
	fmt.Fprintf(b, "    relay_hosts = { %s },\n", strings.Join(parts, ", "))
	b.WriteString("  }\n")
}

// writeEgressPaths emits per-VMTA connection limits. The SOURCE_LIMITS table is
// keyed by egress source (VMTA) name; get_egress_path_config applies the limit.
func writeEgressPaths(b *strings.Builder, vmtas []*VMTA) {
	b.WriteString("-- ===== egress path config (per-VMTA connection limits) =====\n")
	b.WriteString("local SOURCE_LIMITS = {}\n")
	for _, v := range sortedVMTAs(vmtas) {
		if v.Status != VMTAStatusActive && v.Status != VMTAStatusDraining {
			continue
		}
		if v.MaxConnections > 0 {
			fmt.Fprintf(b, "SOURCE_LIMITS[%s] = %d\n", MustLuaString(v.Name), v.MaxConnections)
		}
	}
	b.WriteString(`
kumo.on('get_egress_path_config', function(domain, egress_source, site_name)
  local params = {}
  local limit = SOURCE_LIMITS[egress_source]
  if limit and limit > 0 then
    params.connection_limit = limit
  end
  return kumo.make_egress_path(params)
end)

`)
}

func sortedListeners(in []*Listener) []*Listener {
	out := append([]*Listener(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// luaLogHeaderList builds the configure_log_hook header allow-list: always
// "Subject", plus each distinct header name matched by an active mailclass
// routing rule, rendered as escaped Lua string literals.
func luaLogHeaderList(routes []*RoutingRule) string {
	seen := map[string]struct{}{}
	headers := []string{"Subject"}
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

// fblEnabled reports whether the FBL/ARF feedback pipeline is rendered. The
// log_arf parsing works on its own; the resulting Feedback record only reaches
// iris when the log hook (LogStreamRedisURL) is also configured.
func fblEnabled(snap ConfigSnapshot) bool {
	return strings.TrimSpace(snap.FBLDomain) != ""
}

// writeBounceConsts emits the bounce/DSN + FBL constants (empty when disabled).
func writeBounceConsts(b *strings.Builder, snap ConfigSnapshot) {
	b.WriteString("-- ===== bounce / DSN + FBL pipeline constants =====\n")
	bounceDomain, dsnTracker, dsnStream := "", "", ""
	if bounceEnabled(snap) {
		bounceDomain = strings.ToLower(strings.TrimSpace(snap.BounceDomain))
		dsnTracker = "iris_dsn_catcher"
		dsnStream = DSNStreamName
	}
	fblDomain := ""
	if fblEnabled(snap) {
		fblDomain = strings.ToLower(strings.TrimSpace(snap.FBLDomain))
	}
	fmt.Fprintf(b, "local BOUNCE_DOMAIN = %s\n", MustLuaString(bounceDomain))
	fmt.Fprintf(b, "local DSN_TRACKER   = %s\n", MustLuaString(dsnTracker))
	fmt.Fprintf(b, "local DSN_STREAM    = %s\n", MustLuaString(dsnStream))
	fmt.Fprintf(b, "local FBL_DOMAIN    = %s\n\n", MustLuaString(fblDomain))
}

// webhookEnabled reports whether any active webhook rule with a destination is
// configured (and therefore whether the in-policy webhook poster is rendered).
func webhookEnabled(snap ConfigSnapshot) bool {
	for _, w := range snap.InboundWebhooks {
		if w != nil && w.Status == WebhookActive && strings.TrimSpace(w.DestinationURL) != "" {
			return true
		}
	}
	return false
}

// webhookDomain returns the recipient domain a webhook rule applies to, so the
// listener relays inbound mail for it. For a recipient_email match it is the
// part after '@'.
func webhookDomain(w *WebhookRule) string {
	v := strings.ToLower(strings.TrimSpace(w.MatchValue))
	if w.MatchType == MatchRecipientDomain {
		return v
	}
	if i := strings.LastIndexByte(v, '@'); i >= 0 {
		return v[i+1:]
	}
	return v
}

// writeWebhooks emits the inbound-webhook routing tables and the make.webhook_post
// custom_lua queue constructor. It POSTs the raw RFC822 message (Content-Type
// message/rfc822) with X-Iris-Recipient / X-Iris-Message-Id and, when a secret is
// set, an X-Iris-Signature HMAC-SHA256 of the body — byte-for-byte the same
// request the previous Iris release sent. No-op when no webhook is configured.
func writeWebhooks(b *strings.Builder, snap ConfigSnapshot) {
	if !webhookEnabled(snap) {
		return
	}
	b.WriteString(`-- ===== inbound mail webhooks =====
local WEBHOOK_TRACKER = "iris_webhook"
local WEBHOOK_BY_EMAIL = {}
local WEBHOOK_BY_DOMAIN = {}
local WEBHOOK_DOMAINS = {}
`)
	for _, w := range snap.InboundWebhooks {
		if w == nil || w.Status != WebhookActive || strings.TrimSpace(w.DestinationURL) == "" {
			continue
		}
		entry := fmt.Sprintf("{ url = %s, secret = %s }",
			MustLuaString(w.DestinationURL), MustLuaString(w.SecretRef))
		value := strings.ToLower(strings.TrimSpace(w.MatchValue))
		switch w.MatchType {
		case MatchRecipientEmail:
			fmt.Fprintf(b, "WEBHOOK_BY_EMAIL[%s] = %s\n", MustLuaString(value), entry)
		case MatchRecipientDomain:
			fmt.Fprintf(b, "WEBHOOK_BY_DOMAIN[%s] = %s\n", MustLuaString(value), entry)
		}
		fmt.Fprintf(b, "WEBHOOK_DOMAINS[%s] = true\n", MustLuaString(webhookDomain(w)))
	}
	b.WriteString(`
kumo.on('make.webhook_post', function(_domain, _tenant, _campaign)
  local connection = {}
  function connection:send(message)
    local rcpt = message:recipient()
    local email = (rcpt and rcpt.email or ''):lower()
    local dom = (rcpt and rcpt.domain or ''):lower()
    local route = WEBHOOK_BY_EMAIL[email] or WEBHOOK_BY_DOMAIN[dom]
    if not route then
      return '250 no webhook route (dropped)'
    end
    local body = message:get_data()
    local client = kumo.http.build_client {}
    local req = client:post(route.url)
    req:header('Content-Type', 'message/rfc822')
    req:header('X-Iris-Recipient', email)
    req:header('X-Iris-Message-Id', tostring(message:id()))
    if route.secret and route.secret ~= '' then
      local sig = kumo.digest.hmac_sha256({ key_data = route.secret }, body)
      req:header('X-Iris-Signature', tostring(sig))
    end
    req:body(body)
    local ok, resp = pcall(function() return req:send() end)
    if not ok then
      kumo.log_error('webhook: post failed err=' .. tostring(resp))
      return string.format('451 4.4.1 webhook post error: %s', tostring(resp))
    end
    local code = resp:status_code()
    if code >= 200 and code < 300 then
      return string.format('250 forwarded to webhook (%d)', code)
    end
    return string.format('451 4.4.1 webhook returned %d', code)
  end
  return connection
end)

`)
}

// writeListenerDomain emits the single get_listener_domain handler: it relays
// the bounce domain into the chain (DSN catcher), enables ARF parsing (log_arf)
// for the FBL domain, and relays inbound mail for any webhook domain. Emitted
// when any of those pipelines is configured (the event may be defined once).
func writeListenerDomain(b *strings.Builder, snap ConfigSnapshot) {
	if !bounceEnabled(snap) && !fblEnabled(snap) && !webhookEnabled(snap) {
		return
	}
	b.WriteString(`-- Accept inbound mail for the bounce domain (relayed into the chain, where the
-- reception hook routes it to the DSN tracker), parse ARF reports at the FBL
-- domain (emitting Feedback log records), and relay webhook domains (routed to
-- the webhook poster).
kumo.on('get_listener_domain', function(domain, listener)
  if BOUNCE_DOMAIN ~= '' and domain == BOUNCE_DOMAIN then
    return kumo.make_listener_domain { relay_to = true }
  end
  if FBL_DOMAIN ~= '' and domain == FBL_DOMAIN then
    return kumo.make_listener_domain { log_arf = 'LogThenDrop' }
  end
`)
	if webhookEnabled(snap) {
		b.WriteString(`  if WEBHOOK_DOMAINS[domain] then
    return kumo.make_listener_domain { relay_to = true }
  end
`)
	}
	b.WriteString(`  return nil
end)

`)
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
func writeHostedDomains(b *strings.Builder, snap ConfigSnapshot) {
	b.WriteString("-- ===== hosted (inbound) domains =====\n")
	b.WriteString("local HOSTED_DOMAINS = {}\n")
	for _, d := range snap.hostedDomains() {
		fmt.Fprintf(b, "HOSTED_DOMAINS[%s] = true\n", MustLuaString(d))
	}
	b.WriteString("\n")
}

// writeRspamd emits the iris_rspamd_scan function (or a no-op stub) that scans
// inbound-to-hosted mail through rspamd's /checkv2, adds X-Spam headers, and —
// in enforce mode — honors reject/greylist verdicts. Fail-open throughout.
func writeRspamd(b *strings.Builder, snap ConfigSnapshot) {
	if !snap.rspamdEnabled() {
		b.WriteString("-- ===== inbound spam filtering (rspamd): disabled =====\n")
		b.WriteString("local function iris_rspamd_scan(_msg) end\n\n")
		return
	}
	b.WriteString("-- ===== inbound spam filtering (rspamd) =====\n")
	fmt.Fprintf(b, "local RSPAMD_URL = %s\n", MustLuaString(strings.TrimSpace(snap.RspamdURL)))
	fmt.Fprintf(b, "local RSPAMD_ENFORCE = %t\n", snap.rspamdEnforce())
	b.WriteString(`local function iris_rspamd_scan(msg)
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
  if action == 'reject' then
    if RSPAMD_ENFORCE then
      kumo.reject(550, '5.7.1 message rejected as spam')
      return
    end
    msg:prepend_header('X-Spam', 'yes')
  elseif action == 'soft reject' or action == 'greylist' then
    if RSPAMD_ENFORCE then
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
func sortedSuppressions(in []*SuppressionEntry) []*SuppressionEntry {
	out := append([]*SuppressionEntry(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

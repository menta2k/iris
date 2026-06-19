package kumopolicy

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Snapshot is the input to Render — a structurally validated, post-DB view of
// the policy configuration. Construct it via repository code, then call
// Validate before render.
type Snapshot struct {
	Listeners       []Listener
	DkimIdentities  []DkimIdentity
	VirtualMtas     []VirtualMta
	VirtualMtaGroups []VirtualMtaGroup
	MailClasses     []MailClass
	RoutingRules    []RoutingRule
	Suppressions    []Suppression
	MailWebhooks    []MailWebhook
	GlobalSettings  GlobalSettings
}

// MailWebhook forwards inbound mail for a recipient to an HTTP endpoint.
// Address is an exact recipient ("support@host") or a bare domain ("host")
// catch-all. The renderer emits a lookup table + a custom_lua queue that
// POSTs the raw message to URL (optionally HMAC-signed with Secret).
type MailWebhook struct {
	Name    string
	Address string
	URL     string
	Secret  string
	Enabled bool
}

// WebhookDomain returns the recipient domain this webhook accepts mail for:
// the part after '@' for an exact address, or the whole address for a
// bare-domain catch-all.
func (m MailWebhook) WebhookDomain() string {
	if at := strings.IndexByte(m.Address, '@'); at >= 0 {
		return strings.ToLower(m.Address[at+1:])
	}
	return strings.ToLower(m.Address)
}

// IsDomainCatchAll reports whether the address is a bare domain (no '@').
func (m MailWebhook) IsDomainCatchAll() bool {
	return !strings.Contains(m.Address, "@")
}

// VirtualMtaGroup is the render-friendly view of a weighted VMTA pool. The
// renderer emits a Lua table keyed by group name; routing rules whose target
// kind is "vmta_group" reference these tables for weighted-random selection.
type VirtualMtaGroup struct {
	Name    string
	Enabled bool
	Members []VirtualMtaGroupMember
}

// VirtualMtaGroupMember is one member of a group. Weight 0 disables the
// member without removing it.
type VirtualMtaGroupMember struct {
	VmtaName string
	Weight   uint32
	Priority uint32
	Enabled  bool
}

// GlobalSettings are policy-wide knobs, currently small. Extend cautiously —
// every new field is a new injection surface that needs validation.
type GlobalSettings struct {
	// LogDir is the directory where kumomta will write its log stream. Must be
	// an absolute path with no shell metacharacters.
	LogDir string
	// DiagLogFilter sets kumomta's diagnostic (tracing) log verbosity via
	// kumo.set_diagnostic_log_filter. It accepts tracing-subscriber
	// EnvFilter directives (e.g. "kumod=info,kumod::smtp_server=debug").
	// These diagnostic lines go to stderr/journald, separate from the
	// structured message logs under LogDir. Empty defaults to
	// DiagLogFilterDefault.
	DiagLogFilter string
	// SpoolDir is the directory used for mail spooling.
	SpoolDir string
	// PolicyVersion is a free-form tag that gets embedded as a comment.
	PolicyVersion string
	// MailClassHeader is the header inspected at message reception to look
	// up a MailClass by name. Empty defaults to MailClassHeaderDefault.
	MailClassHeader string

	// EgressEhloDomain is the default outbound EHLO hostname (a FQDN).
	// Rendered as the egress *path* ehlo_domain so all outbound mail
	// announces a resolvable name rather than the bare system hostname
	// (rspamd HFILTER_HELO_5). Per-VMTA HeloName overrides it at the egress
	// *source* level. Also used as the domain for iris-generated
	// Message-IDs. Empty leaves kumomta's default (system hostname).
	EgressEhloDomain string

	// Outbound retry schedule for the normal delivery queue, in
	// KumoMTA/Go duration form ("20m", "4h", "7d"). Empty fields leave
	// kumomta's defaults (retry_interval 20m, doubling, max_age 7d).
	EgressRetryInterval    string
	EgressMaxRetryInterval string
	EgressMaxAge           string

	// Inbound rspamd spam filtering. RspamdMode is "" / "off", "tag"
	// (scan + add X-Spam headers, never reject) or "enforce" (honor
	// rspamd's action incl. reject/defer). RspamdURL is the rspamd HTTP
	// endpoint. Only mail received for hosted domains is scanned.
	RspamdMode string
	RspamdURL  string

	// KumoHTTPListen is the bind spec for kumomta's HTTP admin listener
	// emitted into init.lua's kumo.start_http_listener block. Defaults to
	// '0.0.0.0:8000' (matches the docker-compose layout). Set to
	// '127.0.0.1:8025' or similar for a host-native install where kumomta
	// and admin-service share a host and would otherwise collide on :8000.
	KumoHTTPListen string

	// EsmtpListenAddr is the bind spec emitted into the *default*
	// kumo.start_esmtp_listener block — only consulted when no
	// Listener rows exist on the Listeners page (those rows render as
	// their own per-listener blocks and supersede the default). Empty
	// falls back to "0:2525" so the dev compose stack works without
	// any UI configuration.
	EsmtpListenAddr string

	// EsmtpRelayHosts is the relay-allowed CIDR list emitted into the
	// default kumo.start_esmtp_listener block. Only consulted when no
	// Listener rows exist (per-listener configs override). Empty falls
	// back to the RFC1918 + loopback set so dev compose still works.
	EsmtpRelayHosts []string

	// HTTPTrustedHosts is the trusted-host CIDR list emitted into
	// kumo.start_http_listener. Same semantic as EsmtpRelayHosts: empty
	// = RFC1918 + loopback default.
	HTTPTrustedHosts []string

	// Redis log-hook configuration. When LogStreamRedisURL is non-empty the
	// renderer emits a kumomta log_hook that streams every interesting log
	// record (Reception/Delivery/Bounce/TransientFailure/Feedback) into the
	// named Redis stream via XADD; the admin-service consumes it.
	LogStreamRedisURL string
	LogStreamName     string // default: LogStreamNameDefault
	LogStreamMaxLen   string // default: LogStreamMaxLenDefault — passed verbatim into XADD MAXLEN ~ N

	// TestDomainRoutes overrides the MX lookup for specific recipient
	// domains. Used by the e2e test harness to point fake domains at mock
	// SMTP receivers without standing up a real DNS server. Empty in prod.
	// Format: domain → "host:port" (e.g. "accept.test" → "mock-mta-accept:25").
	TestDomainRoutes map[string]string

	// QueuePerVmta collapses the scheduled-queue keying from `tenant@domain`
	// down to a single-segment queue named after the resolved egress pool
	// (i.e. the VMTA / VMTA-group). One VMTA → one scheduled queue,
	// independent of how many destination domains it sends to.
	//
	// Trade-off: retries for unrelated domains share a queue, so a misbehaving
	// destination can hold up another's retry slot. Off by default.
	QueuePerVmta bool

	// BounceDomain is the receive-side domain (e.g. "bounces.example.com")
	// for **single-domain mode** — every outbound regardless of From: gets
	// rewritten to "b+<token>@<BounceDomain>". Use this when all your
	// sending domains share an organizational domain so DMARC's relaxed
	// alignment treats one bounce subdomain as aligned with all of them.
	//
	// For multi-org sending (e.g. test-1.com AND test2.com from the same
	// instance), use BounceSenderDomains instead — that mode derives a
	// per-sender bounce subdomain by convention so DMARC alignment holds
	// for every From: domain. When BounceSenderDomains is non-empty,
	// BounceDomain is ignored.
	//
	// Empty (and BounceSenderDomains also empty) disables the entire
	// DSN pipeline.
	BounceDomain string

	// BounceSenderDomains lists the From: domains this kumomta hosts
	// outbound for. The renderer emits one bounce subdomain per entry by
	// the convention "<BouncePrefix>.<sender-domain>", so test-1.com
	// becomes bounces.test-1.com (etc.). Outbound mail's MAIL FROM is
	// rewritten to the bounce subdomain matching its From: domain;
	// inbound DSNs are accepted at all of them.
	//
	// Operator must publish DNS MX + SPF for each derived bounce subdomain
	// (see deploy docs).
	BounceSenderDomains []string

	// BouncePrefix is the leading label prepended to each
	// BounceSenderDomains entry to form the bounce subdomain. Default:
	// "bounces" (constant BouncePrefixDefault). Lowercased on use; trailing
	// or leading dots stripped.
	BouncePrefix string

	// VerpSecret is the HMAC key used to sign and verify VERP tokens. Must
	// be at least 16 bytes when BounceDomain is set; otherwise the renderer
	// refuses to emit the VERP rewrite. Never log this value.
	VerpSecret string

	// DsnStreamName is the Redis stream where the kumomta DSN catcher
	// XADDs raw bounce messages. Default: DsnStreamNameDefault. Mirrors
	// the LogStreamName / consumer pattern used for log events.
	DsnStreamName string

	// BounceTokenTTL is the maximum age (since the original send) at which
	// a VERP token is still considered valid. Bounces arriving with older
	// tokens are dropped silently — almost always misdirected mail or
	// backscatter. Empty falls back to BounceTokenTTLDefault.
	BounceTokenTTL string
}

// LogStreamNameDefault is the canonical Redis-stream name. Kept aligned with
// the consumer's default in pkg/logstream so a stock deploy "just works".
const (
	LogStreamNameDefault   = "kumo.events"
	LogStreamMaxLenDefault = "100000"
	logStreamTrackerName   = "iris_logger" // Lua-side log_hook + queue name

	// DSN catcher defaults. Stream name is the Redis stream the kumomta
	// listener XADDs into; the iris consumer (pkg/dsnstream) reads it.
	// Token TTL caps how stale a VERP token can be before the consumer
	// treats it as backscatter.
	DsnStreamNameDefault   = "kumo.dsns"
	dsnTrackerName         = "iris_dsn_catcher" // Lua-side queue + custom_lua name
	BounceTokenTTLDefault  = "720h"             // 30 days
	VerpSecretMinBytes     = 16
	BouncePrefixDefault    = "bounces"

	// DiagLogFilterDefault is the diagnostic (tracing) verbosity emitted
	// when GlobalSettings.DiagLogFilter is empty. Global info with debug on
	// the SMTP server and logging subsystems — the two places operators most
	// often need detail (rejected SMTP commands, log-hook failures) — while
	// keeping overall volume modest. Override via IRIS_KUMO_DIAG_LOG_FILTER.
	DiagLogFilterDefault = "kumod=info,kumod::smtp_server=debug,kumod::logging=debug"
)

// Listener mirrors ListenerConfig but holds only what render needs.
type Listener struct {
	Name           string
	ListenAddr     string // "<ip>:<port>"
	Hostname       string
	TLSEnabled     bool
	TLSCertPath    string // server-side path; presence-checked, not opened
	TLSKeyPath     string
	RequireAuth    bool
	MaxMessageSize uint64
	Domains        []ListenerDomain
}

type ListenerDomain struct {
	Domain       string
	RelayAllowed bool
	RequireTLS   bool
}

type DkimIdentity struct {
	Domain    string
	Selector  string
	Algorithm string // "ed25519" | "rsa-1024" | "rsa-2048" | "rsa-4096"
	KeyPath   string
}

type VirtualMta struct {
	Name                     string
	SourceIPs                []string // already split + validated
	HeloName                 string
	MaxConnections           uint32
	MaxMessagesPerConnection uint32
	ConnectTimeout           uint32 // seconds
	ProviderProfile          string
}

// MailClass is a header-driven routing shortcut. The renderer emits a Lua
// table keyed by class name plus a smtp_server_message_received hook that
// reads the global header (default "X-Kumo-Mail-Class") and resolves the
// class's target — either a VMTA (queue meta) or a VMTA group (resolved
// through pick_vmta_group).
type MailClass struct {
	Name       string
	Enabled    bool
	TargetKind string // "vmta" | "vmta_group"
	TargetRef  string
}

// MailClassHeaderDefault is the global header name used to pick a class at
// reception time. Operators can override via GlobalSettings.MailClassHeader.
const MailClassHeaderDefault = "X-Kumo-Mail-Class"

// RoutingRule is the output-friendly view of routing.
type RoutingRule struct {
	Name       string
	Priority   int32
	Enabled    bool
	Conditions []RuleCondition
	Target     RuleTarget
}

type RuleCondition struct {
	Field string `json:"field"` // allow-list enforced by Validate
	Op    string `json:"op"`
	Value string `json:"value"`
}

type RuleTarget struct {
	Kind       string `json:"kind"` // allow-list enforced by Validate
	Ref        string `json:"ref,omitempty"`
	RejectCode uint32 `json:"reject_code,omitempty"`
	RejectText string `json:"reject_text,omitempty"`
}

// Suppression is shipped to Lua as a fast lookup table.
type Suppression struct {
	Address string
	Scope   string // "address" | "domain"
}

// AllowedConditionFields is the closed set of routing-condition fields. Any
// extension MUST update this list and the corresponding Lua emission.
var AllowedConditionFields = map[string]struct{}{
	"from":           {},
	"to":             {},
	"to_domain":      {},
	"from_domain":    {},
	"header.subject": {},
	"source_ip":      {},
}

// AllowedConditionOps closes the operator set.
var AllowedConditionOps = map[string]struct{}{
	"equals":     {},
	"contains":   {},
	"startswith": {},
	"endswith":   {},
	"regex":      {},
}

// AllowedTargetKinds closes the target set used by routing rules. mail_class
// is intentionally absent: classes resolve at reception via the
// X-Kumo-Mail-Class header, not as an outbound routing target.
var AllowedTargetKinds = map[string]struct{}{
	"vmta":       {},
	"vmta_group": {},
	"queue":      {},
	"reject":     {},
	"discard":    {},
}

// AllowedDkimAlgorithms closes the DKIM algorithm set.
var AllowedDkimAlgorithms = map[string]struct{}{
	"ed25519":   {},
	"rsa-1024":  {},
	"rsa-2048":  {},
	"rsa-4096":  {},
}

// reHostname is a permissive RFC 1123 hostname check (incl. dotted FQDNs).
// Note this is not a TLD verifier — it only rejects characters that could
// escape any context the value is interpolated into.
var reHostname = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,62}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,62}[A-Za-z0-9])?)*$`)

// reSelector matches the DKIM selector grammar (label-like, plus underscores).
var reSelector = regexp.MustCompile(`^[A-Za-z0-9_]([A-Za-z0-9._-]{0,62})?$`)

// reSafePath matches absolute, non-relative paths without shell metas.
// Forbids: spaces, $, `, ;, |, &, newlines, quotes, glob chars, '..'.
var reSafePath = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

// reDiagFilter matches tracing-subscriber EnvFilter directives:
// comma-separated target=level pairs using identifiers, "::" path
// separators, "[span]" qualifiers, and surrounding spaces.
var reDiagFilter = regexp.MustCompile(`^[A-Za-z0-9_,=:.\[\]{} -]+$`)

// reSafeName matches identifiers used as kumomta object names. These are
// rendered as Lua table keys (SOURCES[...], POOLS[...], etc.), so the
// charset stays conservative.
var reSafeName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// reListenerName is looser than reSafeName: a listener's name is a purely
// iris-side label (it is never emitted into the rendered Lua), so operators
// commonly name a listener after its domain — dots are allowed.
var reListenerName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

// ValidationError aggregates one or more issues encountered by Validate.
type ValidationError struct{ Issues []string }

func (v *ValidationError) Error() string {
	if len(v.Issues) == 0 {
		return "kumopolicy: validation failed"
	}
	return "kumopolicy: " + strings.Join(v.Issues, "; ")
}

// Validate returns nil iff the snapshot is safe for Render.
//
// This is the load-bearing check — it is the place where untrusted strings
// get classified safe. We are intentionally strict; if validation rejects a
// legitimate value, fix the rule, never bypass it.
func (s *Snapshot) Validate() error {
	var issues []string
	push := func(format string, a ...any) { issues = append(issues, fmt.Sprintf(format, a...)) }

	// Global settings.
	if s.GlobalSettings.LogDir != "" && !reSafePath.MatchString(s.GlobalSettings.LogDir) {
		push("global.log_dir invalid: %q", s.GlobalSettings.LogDir)
	}
	if s.GlobalSettings.SpoolDir != "" && !reSafePath.MatchString(s.GlobalSettings.SpoolDir) {
		push("global.spool_dir invalid: %q", s.GlobalSettings.SpoolDir)
	}
	// DiagLogFilter is emitted into a Lua string (MustLuaString escapes it),
	// but we still constrain it to the tracing-subscriber EnvFilter alphabet
	// so a typo can't smuggle anything unexpected into the policy.
	if s.GlobalSettings.DiagLogFilter != "" && !reDiagFilter.MatchString(s.GlobalSettings.DiagLogFilter) {
		push("global.diag_log_filter invalid: %q", s.GlobalSettings.DiagLogFilter)
	}
	// KumoHTTPListen flows straight into a Lua string in init.lua's
	// kumo.start_http_listener block; net.SplitHostPort + the empty
	// allowance (renderer applies the default) is enough — anything that
	// reaches kumomta as a non-host:port string fails to bind anyway.
	if s.GlobalSettings.KumoHTTPListen != "" {
		if _, _, err := net.SplitHostPort(s.GlobalSettings.KumoHTTPListen); err != nil {
			push("global.kumo_http_listen invalid: %v", err)
		}
	}

	// Listeners.
	listenerNames := map[string]struct{}{}
	for i, l := range s.Listeners {
		if !reListenerName.MatchString(l.Name) {
			push("listener[%d].name invalid: %q", i, l.Name)
		}
		if _, dup := listenerNames[l.Name]; dup {
			push("listener[%d].name duplicate: %q", i, l.Name)
		}
		listenerNames[l.Name] = struct{}{}
		if _, _, err := net.SplitHostPort(l.ListenAddr); err != nil {
			push("listener[%d].listen_addr invalid: %v", i, err)
		}
		if !reHostname.MatchString(l.Hostname) {
			push("listener[%d].hostname invalid: %q", i, l.Hostname)
		}
		if l.TLSEnabled {
			if !reSafePath.MatchString(l.TLSCertPath) {
				push("listener[%d].tls_cert_pem_path invalid: %q", i, l.TLSCertPath)
			}
			if !reSafePath.MatchString(l.TLSKeyPath) {
				push("listener[%d].tls_key_pem_path invalid: %q", i, l.TLSKeyPath)
			}
		}
		for j, d := range l.Domains {
			if !reHostname.MatchString(d.Domain) {
				push("listener[%d].domains[%d].domain invalid: %q", i, j, d.Domain)
			}
		}
	}

	// DKIM.
	for i, d := range s.DkimIdentities {
		if !reHostname.MatchString(d.Domain) {
			push("dkim[%d].domain invalid: %q", i, d.Domain)
		}
		if !reSelector.MatchString(d.Selector) {
			push("dkim[%d].selector invalid: %q", i, d.Selector)
		}
		if _, ok := AllowedDkimAlgorithms[d.Algorithm]; !ok {
			push("dkim[%d].algorithm invalid: %q", i, d.Algorithm)
		}
		if !reSafePath.MatchString(d.KeyPath) {
			push("dkim[%d].key_path invalid: %q", i, d.KeyPath)
		}
	}

	// VMTAs.
	for i, v := range s.VirtualMtas {
		if !reSafeName.MatchString(v.Name) {
			push("vmta[%d].name invalid: %q", i, v.Name)
		}
		for j, ip := range v.SourceIPs {
			if net.ParseIP(strings.TrimSpace(ip)) == nil {
				push("vmta[%d].source_ips[%d] not an IP: %q", i, j, ip)
			}
		}
		if v.HeloName != "" && !reHostname.MatchString(v.HeloName) {
			push("vmta[%d].helo_name invalid: %q", i, v.HeloName)
		}
		if !reSafeName.MatchString(v.ProviderProfile) {
			push("vmta[%d].provider_profile invalid: %q", i, v.ProviderProfile)
		}
	}

	// MailClasses.
	mailClassTargets := map[string]struct{}{
		"vmta": {}, "vmta_group": {},
	}
	for i, m := range s.MailClasses {
		if !reSafeName.MatchString(m.Name) {
			push("mail_class[%d].name invalid: %q", i, m.Name)
		}
		if !m.Enabled {
			continue
		}
		if _, ok := mailClassTargets[m.TargetKind]; !ok {
			push("mail_class[%d].target_kind invalid: %q (must be vmta or vmta_group)", i, m.TargetKind)
		}
		if !reSafeName.MatchString(m.TargetRef) {
			push("mail_class[%d].target_ref invalid: %q", i, m.TargetRef)
		}
	}

	// Routing.
	for i, r := range s.RoutingRules {
		if r.Name == "" {
			push("routing[%d].name empty", i)
		}
		for j, c := range r.Conditions {
			if _, ok := AllowedConditionFields[c.Field]; !ok {
				push("routing[%d].conditions[%d].field invalid: %q", i, j, c.Field)
			}
			if _, ok := AllowedConditionOps[c.Op]; !ok {
				push("routing[%d].conditions[%d].op invalid: %q", i, j, c.Op)
			}
			if c.Op == "regex" {
				if _, err := regexp.Compile(c.Value); err != nil {
					push("routing[%d].conditions[%d].value bad regex: %v", i, j, err)
				}
			}
		}
		if _, ok := AllowedTargetKinds[r.Target.Kind]; !ok {
			push("routing[%d].target.kind invalid: %q", i, r.Target.Kind)
		}
		if r.Target.Kind == "reject" {
			if r.Target.RejectCode < 400 || r.Target.RejectCode >= 600 {
				push("routing[%d].target.reject_code out of range: %d", i, r.Target.RejectCode)
			}
		}
		if r.Target.Ref != "" && !reSafeName.MatchString(r.Target.Ref) {
			push("routing[%d].target.ref invalid: %q", i, r.Target.Ref)
		}
	}

	// Suppressions.
	for i, sup := range s.Suppressions {
		if sup.Scope != "address" && sup.Scope != "domain" {
			push("suppression[%d].scope invalid: %q", i, sup.Scope)
		}
		if sup.Scope == "domain" && !reHostname.MatchString(sup.Address) {
			push("suppression[%d] domain invalid: %q", i, sup.Address)
		}
		if strings.ContainsAny(sup.Address, "\r\n\x00") {
			push("suppression[%d] address has control chars", i)
		}
	}

	// Mail webhooks (inbound → HTTP). Defence-in-depth — the service layer
	// validates on write; here we guard the render path.
	for i, wh := range s.MailWebhooks {
		if strings.ContainsAny(wh.Address, "\r\n\x00 ") || wh.Address == "" {
			push("mail_webhook[%d].address invalid: %q", i, wh.Address)
		}
		if !reHostname.MatchString(wh.WebhookDomain()) {
			push("mail_webhook[%d] domain invalid: %q", i, wh.WebhookDomain())
		}
		if !strings.HasPrefix(wh.URL, "http://") && !strings.HasPrefix(wh.URL, "https://") {
			push("mail_webhook[%d].url must be http(s): %q", i, wh.URL)
		}
		if strings.ContainsAny(wh.URL, "\r\n\x00 ") {
			push("mail_webhook[%d].url has control chars", i)
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// Errors returned by Render (separately because some are not validation).
var (
	ErrSnapshotInvalid = errors.New("kumopolicy: snapshot invalid")
)

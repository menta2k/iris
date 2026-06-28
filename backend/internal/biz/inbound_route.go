package biz

import (
	"net/url"
	"strings"
)

// Inbound route action types. An InboundRoute matches inbound mail by recipient
// and dispatches it to exactly one of these native kumod queue protocols.
const (
	// InboundActionMaildir writes the message to a Maildir on disk (kumod's
	// maildir_path protocol). The path is the route's MaildirPath when set, else
	// the deployment-wide base plus a per-user template.
	InboundActionMaildir = "maildir"
	// InboundActionForward relays the message to a pinned smarthost (kumod's smtp
	// mx_list protocol), bypassing MX resolution. The envelope recipient is
	// preserved.
	InboundActionForward = "forward"
	// InboundActionWebhook POSTs the raw RFC822 message to DestinationURL
	// (make.webhook_post) — the behaviour formerly owned by WebhookRule.
	InboundActionWebhook = "webhook"
)

// Inbound route status values.
const (
	InboundRouteActive   = "active"
	InboundRouteDisabled = "disabled"
)

// Forward TLS policy values applied to the smarthost delivery path.
const (
	ForwardTLSNone          = "none"          // never use STARTTLS
	ForwardTLSOpportunistic = "opportunistic" // STARTTLS when offered (kumod default)
	ForwardTLSRequired      = "required"      // fail delivery if STARTTLS unavailable
)

// DefaultForwardPort is the smarthost port used when a forward route omits one.
const DefaultForwardPort = 25

// Per-route spam-scan mode. "default" follows the deployment-wide rspamd mode;
// the others override it for this route (honored only when an rspamd URL is
// configured). "off" never scans, "tag" scans and adds X-Spam headers without
// rejecting, "enforce" scans and rejects a spam verdict at SMTP time.
const (
	ScanDefault = "default"
	ScanOff     = "off"
	ScanTag     = "tag"
	ScanEnforce = "enforce"
)

// DefaultWebhookTimeoutSeconds / default retry mirror the legacy WebhookRule
// defaults so backfilled webhook routes behave identically.
const DefaultWebhookTimeoutSeconds = 10

// InboundRoute routes inbound mail for a recipient/domain we are responsible for
// to a maildir, a forwarding smarthost, or an HTTP webhook.
type InboundRoute struct {
	ID         string
	Name       string
	MatchType  string
	MatchValue string
	Action     string
	// Priority breaks ties when several routes could match; higher wins. An exact
	// recipient_email match always outranks a recipient_domain match regardless of
	// priority (resolved at render time).
	Priority int
	Status   string

	// SpamScan controls rspamd scanning for this route's captured mail:
	// "default" (follow the global mode), "off", "tag", or "enforce".
	SpamScan string

	// Forward action.
	ForwardHost string
	ForwardPort int
	ForwardTLS  string

	// Maildir action. Empty => deployment base + "/{{ domain_part }}/{{ local_part }}".
	MaildirPath string

	// Webhook action.
	DestinationURL string
	SecretRef      string
	TimeoutSeconds int
	RetryPolicy    RetryPolicy
}

// Validate normalizes and checks invariants per action. allowInsecure permits a
// plain-HTTP webhook destination for local development; otherwise HTTPS is
// required (same rule as WebhookRule).
func (r *InboundRoute) Validate(allowInsecure bool) error {
	r.Name = strings.TrimSpace(r.Name)
	r.MatchValue = strings.ToLower(strings.TrimSpace(r.MatchValue))
	r.Action = strings.ToLower(strings.TrimSpace(r.Action))
	if r.Status == "" {
		r.Status = InboundRouteActive
	}
	if r.SpamScan == "" {
		r.SpamScan = ScanDefault
	}
	switch r.SpamScan {
	case ScanDefault, ScanOff, ScanTag, ScanEnforce:
	default:
		return Invalid("INBOUND_ROUTE_SPAM_SCAN_INVALID", "spam_scan %q is not valid", r.SpamScan)
	}

	if r.Name == "" {
		return Invalid("INBOUND_ROUTE_NAME_REQUIRED", "route name is required")
	}
	if r.Status != InboundRouteActive && r.Status != InboundRouteDisabled {
		return Invalid("INBOUND_ROUTE_STATUS_INVALID", "status %q is not valid", r.Status)
	}
	if r.MatchType != MatchRecipientEmail && r.MatchType != MatchRecipientDomain {
		return Invalid("INBOUND_ROUTE_MATCH_TYPE_INVALID", "match_type %q is not valid", r.MatchType)
	}
	if r.MatchValue == "" {
		return Invalid("INBOUND_ROUTE_MATCH_VALUE_REQUIRED", "match_value is required")
	}
	if r.MatchType == MatchRecipientEmail && !strings.Contains(r.MatchValue, "@") {
		return Invalid("INBOUND_ROUTE_MATCH_VALUE_INVALID", "recipient_email match_value must be an email address")
	}
	if r.MatchType == MatchRecipientDomain && !dnsNameRe.MatchString(r.MatchValue) {
		return Invalid("INBOUND_ROUTE_MATCH_VALUE_INVALID", "recipient_domain match_value %q is not a valid domain", r.MatchValue)
	}

	switch r.Action {
	case InboundActionMaildir:
		return r.validateMaildir()
	case InboundActionForward:
		return r.validateForward()
	case InboundActionWebhook:
		return r.validateWebhook(allowInsecure)
	default:
		return Invalid("INBOUND_ROUTE_ACTION_INVALID", "action %q is not valid", r.Action)
	}
}

func (r *InboundRoute) validateMaildir() error {
	r.MaildirPath = strings.TrimSpace(r.MaildirPath)
	if r.MaildirPath == "" {
		return nil // deployment base + template is used at render time
	}
	if !strings.HasPrefix(r.MaildirPath, "/") {
		return Invalid("INBOUND_ROUTE_MAILDIR_PATH_INVALID", "maildir_path must be absolute")
	}
	if strings.Contains(r.MaildirPath, "..") {
		return Invalid("INBOUND_ROUTE_MAILDIR_PATH_INVALID", "maildir_path must not contain '..'")
	}
	for _, c := range r.MaildirPath {
		if c < 0x20 {
			return Invalid("INBOUND_ROUTE_MAILDIR_PATH_INVALID", "maildir_path must not contain control characters")
		}
	}
	return nil
}

func (r *InboundRoute) validateForward() error {
	r.ForwardHost = strings.ToLower(strings.TrimSpace(r.ForwardHost))
	if r.ForwardPort == 0 {
		r.ForwardPort = DefaultForwardPort
	}
	if r.ForwardTLS == "" {
		r.ForwardTLS = ForwardTLSOpportunistic
	}
	if r.ForwardHost == "" {
		return Invalid("INBOUND_ROUTE_FORWARD_HOST_REQUIRED", "forward_host is required")
	}
	// A bare hostname or a bracketless IP literal; reject scheme/space/colon (the
	// port is a separate field).
	if strings.ContainsAny(r.ForwardHost, " \t/:") {
		return Invalid("INBOUND_ROUTE_FORWARD_HOST_INVALID", "forward_host %q must be a bare host or IP (set the port separately)", r.ForwardHost)
	}
	if r.ForwardPort < 1 || r.ForwardPort > 65535 {
		return Invalid("INBOUND_ROUTE_FORWARD_PORT_RANGE", "forward_port must be between 1 and 65535")
	}
	switch r.ForwardTLS {
	case ForwardTLSNone, ForwardTLSOpportunistic, ForwardTLSRequired:
	default:
		return Invalid("INBOUND_ROUTE_FORWARD_TLS_INVALID", "forward_tls %q is not valid", r.ForwardTLS)
	}
	return nil
}

func (r *InboundRoute) validateWebhook(allowInsecure bool) error {
	r.DestinationURL = strings.TrimSpace(r.DestinationURL)
	if r.TimeoutSeconds == 0 {
		r.TimeoutSeconds = DefaultWebhookTimeoutSeconds
	}
	if r.RetryPolicy.MaxAttempts == 0 {
		r.RetryPolicy = RetryPolicy{MaxAttempts: 5, BackoffSeconds: 30}
	}
	u, err := url.Parse(r.DestinationURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return Invalid("INBOUND_ROUTE_URL_INVALID", "destination_url %q is not a valid URL", r.DestinationURL)
	}
	if u.Scheme != "https" && !allowInsecure {
		return Invalid("INBOUND_ROUTE_URL_INSECURE", "destination_url must use https")
	}
	if r.TimeoutSeconds < 1 || r.TimeoutSeconds > 120 {
		return Invalid("INBOUND_ROUTE_TIMEOUT_RANGE", "timeout_seconds must be between 1 and 120")
	}
	if r.RetryPolicy.MaxAttempts < 1 || r.RetryPolicy.MaxAttempts > 20 {
		return Invalid("INBOUND_ROUTE_RETRY_RANGE", "max_attempts must be between 1 and 20")
	}
	if strings.Contains(strings.ToUpper(r.SecretRef), "BEGIN") {
		return Invalid("INBOUND_ROUTE_SECRET_INLINE", "secret_ref must be a reference, not inline secret material")
	}
	return nil
}

// RouteDomain returns the recipient domain a route applies to, so the listener
// relays inbound mail for it. For a recipient_email match it is the part after
// '@'.
func (r *InboundRoute) RouteDomain() string {
	v := strings.ToLower(strings.TrimSpace(r.MatchValue))
	if r.MatchType == MatchRecipientDomain {
		return v
	}
	if i := strings.LastIndexByte(v, '@'); i >= 0 {
		return v[i+1:]
	}
	return v
}

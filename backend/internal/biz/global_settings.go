package biz

import (
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Rspamd modes for inbound spam filtering.
const (
	RspamdOff     = "off"
	RspamdTag     = "tag"
	RspamdEnforce = "enforce"
)

// GlobalSettings are the operator-editable, deployment-level policy knobs the
// KumoMTA config generator consumes. They are a singleton (one row). Validation
// is permissive — every field is optional because the generator falls back on
// built-in defaults; the UI is for narrowing those defaults, not re-providing
// them.
type GlobalSettings struct {
	RspamdMode        string
	RspamdURL         string
	EgressEHLODomain  string
	LogStreamRedisURL string
	EsmtpListen       string
	HTTPListen        string

	// Delivery rates: the outbound retry schedule (KumoMTA duration form, e.g.
	// "20m", "4h", "7d"). Empty leaves KumoMTA's defaults.
	EgressRetryInterval    string
	EgressMaxRetryInterval string
	EgressMaxAge           string

	// PinEgressPerMessage keeps a message on one egress IP across retries
	// (deterministic per-message source selection, hashed by message id) instead
	// of KumoMTA's per-attempt weighted round-robin. Default false = round-robin.
	PinEgressPerMessage bool

	// Ipv4Only skips IPv6 MX hosts on outbound delivery (forces IPv4).
	Ipv4Only bool

	// Bounce / DSN pipeline.
	BounceDomain            string
	AutoSuppressHardBounces bool
	SoftBounceThreshold     int

	// BounceDomainTemplate, when set, derives an aligned bounce (VERP
	// return-path) domain per sending domain by substituting "{domain}" — e.g.
	// "bounce.kumo.{domain}" makes mail from @example.com use @bounce.kumo.example.com.
	// This aligns SPF with the From-domain so it backs up DKIM for DMARC. Empty
	// uses the single global BounceDomain for all mail.
	BounceDomainTemplate string

	// FBLRequireVerification gates FBL auto-suppression on provenance: when true,
	// a complainant is suppressed only if the report was proven to be about mail
	// we sent (X-KumoRef trace, send-log, or our DKIM signature). Default false
	// preserves the prior behavior (suppress every complaint).
	FBLRequireVerification bool

	// InboundMaildirBasePath is the deployment-wide Maildir root. Inbound maildir
	// routes with no explicit path land under this base, one Maildir per
	// recipient (<base>/<domain>/<local-part>). Empty falls back to the default.
	InboundMaildirBasePath string

	// SuppressionTTL is the lifetime applied to suppression records (Go/KumoMTA
	// duration form, e.g. "720h", "30d"). Empty = permanent. Enforced as the
	// Redis key TTL on the live suppression list and mirrored to expires_at.
	SuppressionTTL string

	// DMARCReportEmail is the address advertised as the rua= in domains' DMARC
	// records. Inbound aggregate reports arriving here are captured and parsed.
	// Empty disables the DMARC pipeline. One address serves many domains.
	DMARCReportEmail string

	// Iris admin server (applied on restart — the listening socket is bound at
	// startup). AdminHTTPAddr overrides the configured HTTP bind when set. When
	// AdminTLSEnabled, the server serves HTTPS on that address using the issued
	// certificate whose domain is AdminTLSCertDomain; it falls back to plain
	// HTTP if the certificate can't be loaded.
	AdminHTTPAddr      string
	AdminTLSEnabled    bool
	AdminTLSCertDomain string

	// Injection listener (GreenArrow-compatible mail-injection API). Applied on
	// restart — the socket is bound at startup. InjectionEnabled turns the
	// dedicated listener on; InjectionListenAddr/InjectionPath set where it binds
	// and which route it answers; InjectionTLSEnabled serves HTTPS using the
	// issued certificate whose domain is InjectionTLSCertDomain. Credentials are
	// managed separately (the Injection API page).
	InjectionEnabled       bool
	InjectionListenAddr    string
	InjectionPath          string
	InjectionTLSEnabled    bool
	InjectionTLSCertDomain string

	// ACME auto-renew schedule (Go/KumoMTA duration form, e.g. "12h", "30d").
	// Empty uses the env/default (12h scan, renew within 30d of expiry).
	AcmeRenewInterval string
	AcmeRenewBefore   string

	// PrometheusURL is the base URL of the Prometheus that scrapes Iris/KumoMTA
	// (e.g. "http://localhost:9090"). When set, the dashboard metrics endpoint
	// queries it for time-series; empty disables those panels.
	PrometheusURL string

	// Subject classification (optional; off by default). When ClassifySubjects is
	// true, received mail is labeled (≤2 words) via trigram similarity against the
	// subject_classifications corpus, falling back to an OpenAI-compatible model.
	// The API key is supplied via IRIS_OPENAI_API_KEY, never stored here.
	ClassifySubjects  bool
	ClassifyModel     string  // e.g. "gpt-4o-mini"
	ClassifyThreshold float64 // trigram similarity cutoff, 0..1
	ClassifyAPIBase   string  // OpenAI-compatible base URL

	// Inbox-placement monitoring policy. MonitoringFrom is the fallback probe
	// sender for accounts with no from_address. The three durations tune the
	// probe pipeline; empty uses the built-in defaults (1h / 30s / 2h).
	MonitoringFrom              string
	MonitoringReconcileLookback string
	MonitoringFetchTimeout      string
	MonitoringFetchGiveUp       string

	// TLSAutoDisable, when true, lets the log processor automatically add a
	// "Disabled" TLS policy for a destination domain after delivery to it fails a
	// STARTTLS handshake (e.g. a DHE-only server kumod's rustls cannot negotiate),
	// so subsequent mail delivers in cleartext instead of deferring forever.
	// Default false (operator opts in).
	TLSAutoDisable bool

	UpdatedAt time.Time
	UpdatedBy string
}

// Validate normalizes and checks the settings before persistence.
func (g *GlobalSettings) Validate() error {
	g.RspamdMode = strings.ToLower(strings.TrimSpace(g.RspamdMode))
	g.RspamdURL = strings.TrimSpace(g.RspamdURL)
	g.EgressEHLODomain = strings.ToLower(strings.TrimSpace(g.EgressEHLODomain))
	g.LogStreamRedisURL = strings.TrimSpace(g.LogStreamRedisURL)
	g.EsmtpListen = strings.TrimSpace(g.EsmtpListen)
	g.HTTPListen = strings.TrimSpace(g.HTTPListen)

	switch g.RspamdMode {
	case "", RspamdOff, RspamdTag, RspamdEnforce:
	default:
		return Invalid("SETTINGS_RSPAMD_MODE_INVALID", "rspamd_mode must be off, tag, or enforce")
	}
	if g.RspamdMode == RspamdTag || g.RspamdMode == RspamdEnforce {
		if !isHTTPURL(g.RspamdURL) {
			return Invalid("SETTINGS_RSPAMD_URL_INVALID", "rspamd_url must be an http(s):// URL when rspamd is enabled")
		}
	}
	if g.EgressEHLODomain != "" && (len(g.EgressEHLODomain) > 253 || !dnsNameRe.MatchString(g.EgressEHLODomain)) {
		return Invalid("SETTINGS_EHLO_INVALID", "egress_ehlo_domain %q is not a valid DNS name", g.EgressEHLODomain)
	}
	if g.LogStreamRedisURL != "" && !isRedisURL(g.LogStreamRedisURL) {
		return Invalid("SETTINGS_REDIS_URL_INVALID", "log_stream_redis_url must be a redis:// or rediss:// URL")
	}
	if g.EsmtpListen != "" {
		if _, _, err := net.SplitHostPort(g.EsmtpListen); err != nil {
			return Invalid("SETTINGS_ESMTP_LISTEN_INVALID", "esmtp_listen must be host:port")
		}
	}
	if g.HTTPListen != "" {
		if _, _, err := net.SplitHostPort(g.HTTPListen); err != nil {
			return Invalid("SETTINGS_HTTP_LISTEN_INVALID", "http_listen must be host:port")
		}
	}

	// Delivery rates: validate each duration (KumoMTA form).
	g.EgressRetryInterval = strings.TrimSpace(g.EgressRetryInterval)
	g.EgressMaxRetryInterval = strings.TrimSpace(g.EgressMaxRetryInterval)
	g.EgressMaxAge = strings.TrimSpace(g.EgressMaxAge)
	for field, v := range map[string]string{
		"egress_retry_interval":     g.EgressRetryInterval,
		"egress_max_retry_interval": g.EgressMaxRetryInterval,
		"egress_max_age":            g.EgressMaxAge,
	} {
		if v != "" && !kumoDurationRe.MatchString(v) {
			return Invalid("SETTINGS_DURATION_INVALID", "%s %q is not a valid duration (e.g. 20m, 4h, 7d)", field, v)
		}
	}

	// Bounce / DSN pipeline.
	g.BounceDomain = SanitizeAddress(g.BounceDomain)
	if g.BounceDomain != "" && (len(g.BounceDomain) > 253 || !dnsNameRe.MatchString(g.BounceDomain)) {
		return Invalid("SETTINGS_BOUNCE_DOMAIN_INVALID", "bounce_domain %q is not a valid DNS name", g.BounceDomain)
	}
	// Per-sending-domain bounce template (optional). When set it must contain the
	// {domain} placeholder and, with the placeholder substituted, expand to a
	// valid DNS name — so every derived per-domain bounce domain is well-formed.
	g.BounceDomainTemplate = strings.ToLower(strings.TrimSpace(g.BounceDomainTemplate))
	if g.BounceDomainTemplate != "" {
		if !strings.Contains(g.BounceDomainTemplate, BounceDomainPlaceholder) {
			return Invalid("SETTINGS_BOUNCE_TEMPLATE_INVALID",
				"bounce_domain_template %q must contain the %s placeholder", g.BounceDomainTemplate, BounceDomainPlaceholder)
		}
		sample := strings.ReplaceAll(g.BounceDomainTemplate, BounceDomainPlaceholder, "example.com")
		if len(sample) > 253 || !dnsNameRe.MatchString(sample) {
			return Invalid("SETTINGS_BOUNCE_TEMPLATE_INVALID",
				"bounce_domain_template %q does not expand to a valid DNS name", g.BounceDomainTemplate)
		}
	}
	if g.SoftBounceThreshold < 0 || g.SoftBounceThreshold > 1000 {
		return Invalid("SETTINGS_SOFT_THRESHOLD_RANGE", "soft_bounce_threshold must be between 0 and 1000")
	}
	// Suppression record lifetime (optional duration; empty = permanent).
	g.SuppressionTTL = strings.TrimSpace(g.SuppressionTTL)
	if g.SuppressionTTL != "" && !kumoDurationRe.MatchString(g.SuppressionTTL) {
		return Invalid("SETTINGS_DURATION_INVALID", "suppression_ttl %q is not a valid duration (e.g. 720h, 30d)", g.SuppressionTTL)
	}
	// DMARC aggregate-report address (optional; must be a valid email when set).
	// SanitizeAddress (not just TrimSpace) strips zero-width / format runes that
	// copy-paste injects; a hidden rune here would otherwise be stored, render
	// into the policy, and silently break the reception-hook catcher.
	g.DMARCReportEmail = SanitizeAddress(g.DMARCReportEmail)
	if g.DMARCReportEmail != "" && !isValidEmail(g.DMARCReportEmail) {
		return Invalid("SETTINGS_DMARC_EMAIL_INVALID", "dmarc_report_email %q is not a valid email address", g.DMARCReportEmail)
	}

	// Iris admin server.
	g.AdminHTTPAddr = strings.TrimSpace(g.AdminHTTPAddr)
	if g.AdminHTTPAddr != "" {
		if _, _, err := net.SplitHostPort(g.AdminHTTPAddr); err != nil {
			return Invalid("SETTINGS_ADMIN_ADDR_INVALID", "admin_http_addr must be host:port (e.g. :8080)")
		}
	}
	g.AdminTLSCertDomain = strings.ToLower(strings.TrimSpace(g.AdminTLSCertDomain))
	if g.AdminTLSEnabled && g.AdminTLSCertDomain == "" {
		return Invalid("SETTINGS_ADMIN_TLS_CERT_REQUIRED", "admin_tls_cert_domain is required when admin TLS is enabled")
	}

	// Injection listener (applied on restart). Defaults fill in when enabled and
	// left blank; TLS requires a certificate domain.
	g.InjectionListenAddr = strings.TrimSpace(g.InjectionListenAddr)
	if g.InjectionListenAddr != "" {
		if _, _, err := net.SplitHostPort(g.InjectionListenAddr); err != nil {
			return Invalid("SETTINGS_INJECTION_ADDR_INVALID", "injection_listen_addr must be host:port (e.g. :8025)")
		}
	}
	g.InjectionPath = strings.TrimSpace(g.InjectionPath)
	if g.InjectionPath != "" && !strings.HasPrefix(g.InjectionPath, "/") {
		return Invalid("SETTINGS_INJECTION_PATH_INVALID", "injection_path must start with '/'")
	}
	g.InjectionTLSCertDomain = strings.ToLower(strings.TrimSpace(g.InjectionTLSCertDomain))
	if g.InjectionEnabled {
		if g.InjectionPath == "" {
			g.InjectionPath = "/api/inject"
		}
		if g.InjectionTLSEnabled && g.InjectionTLSCertDomain == "" {
			return Invalid("SETTINGS_INJECTION_TLS_CERT_REQUIRED", "injection_tls_cert_domain is required when injection TLS is enabled")
		}
	}

	// Prometheus base URL (optional; must be http(s) when set).
	g.PrometheusURL = strings.TrimSpace(g.PrometheusURL)
	if g.PrometheusURL != "" && !isHTTPURL(g.PrometheusURL) {
		return Invalid("SETTINGS_PROMETHEUS_URL_INVALID", "prometheus_url must be an http(s):// URL")
	}

	// Subject classification knobs (defaults applied when empty; the feature is
	// gated by ClassifySubjects and the IRIS_OPENAI_API_KEY env var).
	g.ClassifyModel = strings.TrimSpace(g.ClassifyModel)
	if g.ClassifyModel == "" {
		g.ClassifyModel = "gpt-4o-mini"
	}
	g.ClassifyAPIBase = strings.TrimSpace(g.ClassifyAPIBase)
	if g.ClassifyAPIBase == "" {
		g.ClassifyAPIBase = "https://api.openai.com/v1"
	}
	if !isHTTPURL(g.ClassifyAPIBase) {
		return Invalid("SETTINGS_CLASSIFY_API_BASE_INVALID", "classify_api_base must be an http(s):// URL")
	}
	if g.ClassifyThreshold <= 0 {
		g.ClassifyThreshold = 0.45
	}
	if g.ClassifyThreshold > 1 {
		return Invalid("SETTINGS_CLASSIFY_THRESHOLD_RANGE", "classify_threshold must be between 0 and 1")
	}

	// Inbox-monitoring policy. The sender must be a valid email when set; each
	// tuning value must be a valid duration when set (empty = built-in default).
	g.MonitoringFrom = SanitizeAddress(g.MonitoringFrom)
	if g.MonitoringFrom != "" && !isValidEmail(g.MonitoringFrom) {
		return Invalid("SETTINGS_MONITORING_FROM_INVALID", "monitoring_from %q is not a valid email address", g.MonitoringFrom)
	}
	g.MonitoringReconcileLookback = strings.TrimSpace(g.MonitoringReconcileLookback)
	g.MonitoringFetchTimeout = strings.TrimSpace(g.MonitoringFetchTimeout)
	g.MonitoringFetchGiveUp = strings.TrimSpace(g.MonitoringFetchGiveUp)
	for field, v := range map[string]string{
		"monitoring_reconcile_lookback": g.MonitoringReconcileLookback,
		"monitoring_fetch_timeout":      g.MonitoringFetchTimeout,
		"monitoring_fetch_giveup":       g.MonitoringFetchGiveUp,
	} {
		if v != "" && !kumoDurationRe.MatchString(v) {
			return Invalid("SETTINGS_DURATION_INVALID", "%s %q is not a valid duration (e.g. 30s, 1h, 2h)", field, v)
		}
	}

	// ACME renew schedule.
	g.AcmeRenewInterval = strings.TrimSpace(g.AcmeRenewInterval)
	g.AcmeRenewBefore = strings.TrimSpace(g.AcmeRenewBefore)
	for field, v := range map[string]string{
		"acme_renew_interval": g.AcmeRenewInterval,
		"acme_renew_before":   g.AcmeRenewBefore,
	} {
		if v != "" && !kumoDurationRe.MatchString(v) {
			return Invalid("SETTINGS_DURATION_INVALID", "%s %q is not a valid duration (e.g. 12h, 30d)", field, v)
		}
	}
	return nil
}

// ParseFlexDuration parses a duration in Go/KumoMTA form, additionally
// supporting a "d" (day = 24h) unit that time.ParseDuration rejects. Empty or
// invalid input returns (0, false).
func ParseFlexDuration(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if s == "" || !kumoDurationRe.MatchString(s) {
		return 0, false
	}
	var total time.Duration
	num := 0
	seen := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			num = num*10 + int(r-'0')
			seen = true
		default:
			unit := map[rune]time.Duration{
				's': time.Second, 'm': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour,
			}[r]
			total += time.Duration(num) * unit
			num = 0
		}
	}
	if !seen {
		return 0, false
	}
	return total, true
}

// kumoDurationRe matches a KumoMTA/Go-ish duration: one or more <number><unit>
// segments where unit is s, m, h, or d (e.g. "20m", "4h", "7d", "1h30m").
var kumoDurationRe = regexp.MustCompile(`^(\d+(s|m|h|d))+$`)

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func isRedisURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "redis" || u.Scheme == "rediss") && u.Host != ""
}

package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// GlobalSettingsRow is the operator-editable subset of kumopolicy.GlobalSettings.
// Excludes infra/secret values (VERP secret, Redis URL, JWT keys, DB DSN)
// — those stay env-only and are wired separately in providers.
type GlobalSettingsRow struct {
	KumoHTTPListen      string
	EsmtpListenAddr     string
	EsmtpRelayHosts     []string
	HTTPTrustedHosts    []string
	BounceDomain        string
	BounceSenderDomains []string
	BouncePrefix        string
	MailClassHeader     string
	EgressEhloDomain    string

	EgressRetryInterval    string
	EgressMaxRetryInterval string
	EgressMaxAge           string

	RspamdMode string
	RspamdURL  string

	// Iris admin HTTPS — a TLS-terminating reverse proxy that fronts
	// the plain :8000 server. Empty Listen disables.
	HTTPSListen      string
	HTTPSCertPemPath string
	HTTPSKeyPemPath  string

	UpdatedAt time.Time
	UpdatedBy string
}

// GlobalSettingsStore is the data-layer contract.
type GlobalSettingsStore interface {
	// Get returns the singleton row. Implementations seed the row on
	// first access so callers never see a not-found error.
	Get(ctx context.Context) (*GlobalSettingsRow, error)
	// Update writes every field on the input row, regardless of zero
	// values — the UI sends the complete state on save and "clear this
	// field" must round-trip.
	Update(ctx context.Context, in GlobalSettingsRow, actor string) (*GlobalSettingsRow, error)
}

// GlobalSettingsService applies validation + audit metadata around the
// store. The renderer doesn't depend on it directly; the snapshot
// provider does (see SnapshotProvider).
//
// Update notifies registered observers after a successful write so
// live-reconfigurable consumers (e.g. the HTTPS listener, which owns a
// bound socket derived from these settings) can re-read and re-apply
// without a process restart. Observers are fired on detached goroutines
// — a callback must not block the caller's request, and one that owns
// the connection the request arrived on (the HTTPS proxy) would
// otherwise deadlock draining itself.
type GlobalSettingsService struct {
	store GlobalSettingsStore

	mu       sync.Mutex
	onChange []func()
}

func NewGlobalSettingsService(s GlobalSettingsStore) *GlobalSettingsService {
	return &GlobalSettingsService{store: s}
}

// OnChange registers a callback invoked after every successful Update.
// Registration is expected during boot (single goroutine), before the
// server starts accepting mutations. Nil callbacks are ignored.
func (s *GlobalSettingsService) OnChange(fn func()) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	s.onChange = append(s.onChange, fn)
	s.mu.Unlock()
}

// fireOnChange invokes every registered observer on its own goroutine.
// Detached on purpose — see the type comment.
func (s *GlobalSettingsService) fireOnChange() {
	s.mu.Lock()
	fns := append([]func(){}, s.onChange...)
	s.mu.Unlock()
	for _, fn := range fns {
		go fn()
	}
}

// Get returns the current settings, normalised: list fields are
// de-duped and lowercased domains pass through parseDomainList-style
// cleanup so renderer output matches what the UI showed.
func (s *GlobalSettingsService) Get(ctx context.Context) (*GlobalSettingsRow, error) {
	row, err := s.store.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("global_settings: get: %w", err)
	}
	normaliseRow(row)
	return row, nil
}

// Update validates the input and persists it. Validation is permissive
// (no field is required) because the renderer is built to fall back on
// hardcoded defaults — the UI is for *narrowing* the defaults, not
// re-providing them.
func (s *GlobalSettingsService) Update(ctx context.Context, in GlobalSettingsRow, actor string) (*GlobalSettingsRow, error) {
	if err := validateGlobalSettings(&in); err != nil {
		return nil, err
	}
	normaliseRow(&in)
	row, err := s.store.Update(ctx, in, actor)
	if err != nil {
		return nil, fmt.Errorf("global_settings: update: %w", err)
	}
	normaliseRow(row)
	s.fireOnChange()
	return row, nil
}

// validateGlobalSettings catches the obvious user-input mistakes. We
// don't try to validate CIDR / hostname syntax here — kumomta's policy
// validator runs at render-Apply time and surfaces those errors with
// kumomta's own diagnostics, which are friendlier than anything we'd
// produce out of band.
func validateGlobalSettings(in *GlobalSettingsRow) error {
	// kumo_http_listen "host:port" cheap shape check; full bind validation
	// happens when kumomta tries to parse it.
	if in.KumoHTTPListen != "" && !strings.Contains(in.KumoHTTPListen, ":") {
		return errors.New("kumo_http_listen must be host:port (e.g. 0.0.0.0:8000)")
	}
	if in.EsmtpListenAddr != "" && !strings.Contains(in.EsmtpListenAddr, ":") {
		return errors.New("esmtp_listen_addr must be host:port (e.g. 0:25 or 0.0.0.0:2525)")
	}
	if in.HTTPSListen != "" {
		if !strings.Contains(in.HTTPSListen, ":") {
			return errors.New("https_listen must be host:port (e.g. :443)")
		}
		if in.HTTPSCertPemPath == "" || in.HTTPSKeyPemPath == "" {
			return errors.New("https_cert_pem_path and https_key_pem_path are required when https_listen is set")
		}
	}
	// Multi-domain mode requires at least one entry; setting an empty
	// list with a non-empty single domain is fine (legacy mode).
	for _, d := range in.BounceSenderDomains {
		if strings.ContainsAny(d, " \t\r\n") {
			return errors.New("bounce_sender_domains entries must not contain whitespace")
		}
	}
	for _, f := range []struct{ name, val string }{
		{"egress_retry_interval", in.EgressRetryInterval},
		{"egress_max_retry_interval", in.EgressMaxRetryInterval},
		{"egress_max_age", in.EgressMaxAge},
	} {
		if !isValidDuration(f.val) {
			return fmt.Errorf("%s must be a duration like 20m, 4h or 7d", f.name)
		}
	}
	// rspamd inbound spam filtering.
	mode := strings.ToLower(strings.TrimSpace(in.RspamdMode))
	switch mode {
	case "", "off", "tag", "enforce":
	default:
		return errors.New("rspamd_mode must be off, tag or enforce")
	}
	url := strings.TrimSpace(in.RspamdURL)
	if mode == "tag" || mode == "enforce" {
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return errors.New("rspamd_url must be an http(s):// URL when rspamd is enabled")
		}
	}
	return nil
}

// reDuration matches a simple single-unit duration (the common KumoMTA
// forms). Empty is allowed by isValidDuration (= use kumomta default).
var reDuration = regexp.MustCompile(`^[0-9]+(ms|s|m|h|d)$`)

func isValidDuration(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return true
	}
	return reDuration.MatchString(v)
}

// normaliseRow runs the same cleanup the env parser does so the
// renderer output matches what the UI showed: lowercase + trim + dedup
// for list fields.
func normaliseRow(r *GlobalSettingsRow) {
	if r == nil {
		return
	}
	r.KumoHTTPListen = strings.TrimSpace(r.KumoHTTPListen)
	r.EsmtpListenAddr = strings.TrimSpace(r.EsmtpListenAddr)
	r.EsmtpRelayHosts = dedupTrim(r.EsmtpRelayHosts, false)
	r.HTTPTrustedHosts = dedupTrim(r.HTTPTrustedHosts, false)
	r.BounceDomain = strings.ToLower(strings.TrimSpace(r.BounceDomain))
	r.BounceSenderDomains = dedupTrim(r.BounceSenderDomains, true)
	r.BouncePrefix = strings.Trim(strings.ToLower(strings.TrimSpace(r.BouncePrefix)), ".")
	r.MailClassHeader = strings.TrimSpace(r.MailClassHeader)
	r.EgressEhloDomain = strings.ToLower(strings.TrimSpace(r.EgressEhloDomain))
	r.EgressRetryInterval = strings.ToLower(strings.TrimSpace(r.EgressRetryInterval))
	r.EgressMaxRetryInterval = strings.ToLower(strings.TrimSpace(r.EgressMaxRetryInterval))
	r.EgressMaxAge = strings.ToLower(strings.TrimSpace(r.EgressMaxAge))
	r.RspamdMode = strings.ToLower(strings.TrimSpace(r.RspamdMode))
	r.RspamdURL = strings.TrimSpace(r.RspamdURL)
	r.HTTPSListen = strings.TrimSpace(r.HTTPSListen)
	r.HTTPSCertPemPath = strings.TrimSpace(r.HTTPSCertPemPath)
	r.HTTPSKeyPemPath = strings.TrimSpace(r.HTTPSKeyPemPath)
}

// dedupTrim trims whitespace, optionally lowercases (domain-style
// fields), drops empties, and removes duplicates. Order-preserving so
// operator intent is visible in the rendered policy.
func dedupTrim(in []string, lower bool) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if lower {
			v = strings.Trim(strings.ToLower(v), ".")
		}
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

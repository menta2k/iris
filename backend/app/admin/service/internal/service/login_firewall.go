// LoginFirewall evaluates login-firewall rules against a login attempt.
//
// Semantics (default ALLOW):
//  1. Deny precedence: the first matching BLACKLIST rule denies.
//  2. Whitelist restriction, per method: for each method that has at least
//     one WHITELIST rule, the attempt's value for that method must match at
//     least one of them; otherwise deny. Methods with no whitelist rules are
//     unrestricted.
//
// Fail-open per attribute: when an attribute can't be determined (IP empty /
// unparseable, geo disabled or lookup error, time-window misconfigured), the
// rules of that method are skipped and a warning is logged. This is a
// deliberate availability-over-strictness bias for an admin console: a
// stripped X-Forwarded-For or a missing GeoIP database must not lock everyone
// out. The tradeoff — a proxy that drops the client IP silently disables IP
// and REGION enforcement — is made observable by the WARN logs.
package service

import (
	"context"
	"log"
	"net"
	"slices"
	"strings"
	"time"
)

// GeoResolver resolves an IP to an ISO-3166-1 alpha-2 country code. It
// returns ("", nil) when the country is indeterminate (geo disabled, private
// IP, unresolvable) so the firewall fails open; ("", err) signals a real
// lookup failure (also treated as indeterminate by the evaluator).
type GeoResolver interface {
	CountryISO(ip string) (string, error)
}

// RuleSource supplies the applicable rules for an attempt (the repo).
type RuleSource interface {
	ListApplicable(ctx context.Context, userID *uint32) ([]LoginPolicyRow, error)
}

// LoginAttempt is the input to a firewall evaluation.
type LoginAttempt struct {
	Username string
	UserID   *uint32 // nil for a non-existent user (global rules only)
	IP       string
	Now      time.Time
}

// FirewallResult is the verdict. Reason is intentionally generic on deny so
// it doesn't leak which rule fired.
type FirewallResult struct {
	Allowed       bool
	Reason        string
	MatchedRuleID uint32
}

// LoginFirewall enforces login policies. A nil firewall is never used by the
// auth path (the field is nil-checked there); geo may be nil (REGION rules
// then fail open).
type LoginFirewall struct {
	rules RuleSource
	geo   GeoResolver
}

// NewLoginFirewall constructs the firewall. geo may be nil.
func NewLoginFirewall(rules RuleSource, geo GeoResolver) *LoginFirewall {
	return &LoginFirewall{rules: rules, geo: geo}
}

// Evaluate loads the applicable rules and applies the firewall semantics.
// A store error is returned to the caller, which must fail open (a DB blip
// can't be allowed to block all logins).
func (f *LoginFirewall) Evaluate(ctx context.Context, a LoginAttempt) (FirewallResult, error) {
	rules, err := f.rules.ListApplicable(ctx, a.UserID)
	if err != nil {
		return FirewallResult{Allowed: true}, err
	}
	return evaluateRules(rules, a, f.geo), nil
}

// matchDecision is the per-rule outcome for the attempt's attribute.
type matchDecision int

const (
	decNoMatch matchDecision = iota
	decMatch
	decIndeterminate
)

// evaluateRules is the pure decision function shared by the firewall and the
// self-lockout guard so the two never diverge. geo may be nil.
func evaluateRules(rules []LoginPolicyRow, a LoginAttempt, geo GeoResolver) FirewallResult {
	// 1. Deny precedence — any matching blacklist wins.
	for i := range rules {
		r := &rules[i]
		if r.Type != PolicyTypeBlacklist {
			continue
		}
		switch matchRule(r, a, geo) {
		case decMatch:
			return FirewallResult{Allowed: false, Reason: "blocked by login policy", MatchedRuleID: r.ID}
		case decIndeterminate:
			log.Printf("login_firewall: blacklist rule %d (%s) indeterminate for ip=%q — failing open", r.ID, r.Method, a.IP)
		}
	}

	// 2. Whitelist restriction, grouped per method.
	type group struct {
		satisfied      bool
		anyDeterminate bool
		firstRuleID    uint32
		method         string
	}
	groups := map[string]*group{}
	for i := range rules {
		r := &rules[i]
		if r.Type != PolicyTypeWhitelist {
			continue
		}
		g := groups[r.Method]
		if g == nil {
			g = &group{firstRuleID: r.ID, method: r.Method}
			groups[r.Method] = g
		}
		switch matchRule(r, a, geo) {
		case decMatch:
			g.satisfied = true
			g.anyDeterminate = true
		case decNoMatch:
			g.anyDeterminate = true
		case decIndeterminate:
			log.Printf("login_firewall: whitelist rule %d (%s) indeterminate for ip=%q — failing open", r.ID, r.Method, a.IP)
		}
	}
	for _, g := range groups {
		if g.satisfied {
			continue
		}
		if !g.anyDeterminate {
			// Every whitelist rule for this method was indeterminate —
			// fail open rather than lock the user out on a geo/proxy hiccup.
			continue
		}
		return FirewallResult{Allowed: false, Reason: "login not permitted from this context", MatchedRuleID: g.firstRuleID}
	}

	return FirewallResult{Allowed: true}
}

// matchRule reports whether the attempt matches a single rule's criterion.
func matchRule(r *LoginPolicyRow, a LoginAttempt, geo GeoResolver) matchDecision {
	switch r.Method {
	case MethodIP:
		return matchIP(r.Value, a.IP)
	case MethodRegion:
		return matchRegion(r.Value, a.IP, geo)
	case MethodTime:
		return matchTime(r.TimeWindow, a.Now)
	default:
		// MAC / DEVICE / unknown — never enforced.
		return decIndeterminate
	}
}

func matchIP(ruleValue, clientIP string) matchDecision {
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil {
		return decIndeterminate
	}
	ipnet, err := parseRuleCIDR(ruleValue)
	if err != nil {
		return decIndeterminate
	}
	if ipnet.Contains(ip) {
		return decMatch
	}
	return decNoMatch
}

func matchRegion(ruleValue, clientIP string, geo GeoResolver) matchDecision {
	if geo == nil {
		return decIndeterminate
	}
	code, err := geo.CountryISO(clientIP)
	if err != nil || code == "" {
		return decIndeterminate
	}
	if strings.EqualFold(code, ruleValue) {
		return decMatch
	}
	return decNoMatch
}

func matchTime(tw *TimeWindow, now time.Time) matchDecision {
	if tw == nil {
		return decIndeterminate
	}
	start, err := parseHHMM(tw.Start)
	if err != nil {
		return decIndeterminate
	}
	end, err := parseHHMM(tw.End)
	if err != nil || start > end {
		return decIndeterminate // wrap-past-midnight unsupported in v1
	}
	loc := time.UTC
	if tw.Timezone != "" {
		l, err := time.LoadLocation(tw.Timezone)
		if err != nil {
			return decIndeterminate
		}
		loc = l
	}
	local := now.In(loc)
	if len(tw.Days) > 0 && !slices.Contains(tw.Days, local.Weekday()) {
		return decNoMatch
	}
	mins := local.Hour()*60 + local.Minute()
	if mins >= start && mins <= end {
		return decMatch
	}
	return decNoMatch
}

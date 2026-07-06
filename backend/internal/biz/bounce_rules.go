package biz

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Bounce-action rule actions. These map onto engines iris already runs: SUPPRESS
// adds the recipient to the suppression list (at bounce ingestion); THROTTLE and
// SUSPEND_DOMAIN are compiled into KumoMTA Traffic Shaping Automation rules;
// RETRY is the default (informational — no override).
const (
	BounceActionRetry         = "retry"
	BounceActionThrottle      = "throttle"
	BounceActionSuspendDomain = "suspend_domain"
	BounceActionSuppress      = "suppress"
)

// bounceActions is the set of valid actions.
var bounceActions = map[string]struct{}{
	BounceActionRetry: {}, BounceActionThrottle: {},
	BounceActionSuspendDomain: {}, BounceActionSuppress: {},
}

// Bounce classes.
const (
	BounceClassSoft = "soft"
	BounceClassHard = "hard"
)

// Rule sources: seeded defaults vs operator overlay.
const (
	BounceRuleSourceDefault = "default"
	BounceRuleSourceOverlay = "overlay"
)

// BounceActionRule maps a bounce signature (SMTP code + enhanced code + provider
// + diagnostic pattern) to a category and a system action. Empty match fields
// are wildcards. Higher Priority wins; ties break toward more specific rules.
type BounceActionRule struct {
	ID           string
	SMTPCode     string // "421", "550", or "" (any)
	EnhancedCode string // "4.7.0", "5.1.1", or "" (any)
	Provider     string // gmail | yahoo | microsoft | ... or "" (all)
	Pattern      string // case-insensitive substring matched in the diagnostic
	Class        string // soft | hard
	Category     string // human category, e.g. "Rate Limited"
	Action       string // retry | throttle | suspend_domain | suppress
	// ActionConfig parameterises the action: throttle → a rate like "100/h";
	// suspend_domain → a hold duration like "2h". Ignored for retry/suppress.
	ActionConfig    string
	SuggestedAction string // operator-facing guidance shown in the console
	Priority        int    // higher wins
	// MinAttempts gates the rule on the message's delivery-attempt count: the rule
	// only applies once the message has been tried at least this many times. 0
	// means it applies on the first matching event. Used to suppress a recipient
	// only after repeated transient failures (e.g. a persistently-full mailbox).
	MinAttempts int
	// SuppressTTL, for a suppress action, is how long the recipient stays
	// suppressed (KumoMTA duration form, e.g. "30d"). Empty uses the global
	// suppression TTL. Lets, e.g., invalid-recipient suppress permanently while a
	// full-mailbox suppress lapses after a while so the address can be retried.
	SuppressTTL string
	Source      string // default | overlay
	Status      string // active | disabled
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// enhancedCodeRe extracts an RFC 3463 enhanced status code (x.y.z) from a
// diagnostic string, e.g. "550 5.1.1 User unknown" → "5.1.1".
var enhancedCodeRe = regexp.MustCompile(`\b([245])\.\d{1,3}\.\d{1,3}\b`)

// ParseEnhancedCode returns the enhanced status code embedded in a diagnostic,
// or "" when none is present.
func ParseEnhancedCode(diagnostic string) string {
	return enhancedCodeRe.FindString(diagnostic)
}

// providerByDomainSuffix maps a recipient domain (or MX-ish suffix) to a
// normalized provider bucket used for rule matching.
var providerByDomainSuffix = []struct{ suffix, provider string }{
	{"gmail.com", "gmail"},
	{"googlemail.com", "gmail"},
	{"google.com", "gmail"},
	{"yahoo.com", "yahoo"},
	{"yahoo.co", "yahoo"},
	{"ymail.com", "yahoo"},
	{"aol.com", "yahoo"},
	{"outlook.com", "microsoft"},
	{"hotmail.com", "microsoft"},
	{"live.com", "microsoft"},
	{"msn.com", "microsoft"},
	{"office365.com", "microsoft"},
	{"icloud.com", "apple"},
	{"me.com", "apple"},
	{"mac.com", "apple"},
	{"mail.ru", "mailru"},
	{"proton.me", "proton"},
	{"protonmail.com", "proton"},
}

// ProviderForDomain classifies a recipient domain into a provider bucket, or ""
// when it maps to no known provider (a rule with Provider="" still matches it).
func ProviderForDomain(domain string) string {
	d := strings.ToLower(strings.TrimSpace(domain))
	if d == "" {
		return ""
	}
	for _, p := range providerByDomainSuffix {
		if d == p.suffix || strings.HasSuffix(d, "."+p.suffix) {
			return p.provider
		}
	}
	return ""
}

// BounceSignature is the normalized input the matcher evaluates a bounce against.
type BounceSignature struct {
	SMTPCode     string // "550"
	EnhancedCode string // "5.1.1" (empty ok; derived from Diagnostic if blank)
	Provider     string // normalized provider (empty ok; derived from Domain)
	Domain       string // recipient domain (used to derive Provider when blank)
	Diagnostic   string // full server response text
	Attempts     int    // delivery attempts so far (gates rules' MinAttempts)
}

// normalize fills in EnhancedCode/Provider from the diagnostic/domain when blank
// and lowercases the diagnostic for pattern matching.
func (s BounceSignature) normalize() BounceSignature {
	if s.EnhancedCode == "" {
		s.EnhancedCode = ParseEnhancedCode(s.Diagnostic)
	}
	if s.Provider == "" {
		s.Provider = ProviderForDomain(s.Domain)
	}
	return s
}

// matches reports whether the rule applies to the signature. Empty rule match
// fields are wildcards; Pattern is a case-insensitive substring of the diagnostic.
func (r *BounceActionRule) matches(sig BounceSignature) bool {
	if r.Status != "" && r.Status != "active" {
		return false
	}
	if r.MinAttempts > 0 && sig.Attempts < r.MinAttempts {
		return false
	}
	if r.SMTPCode != "" && r.SMTPCode != sig.SMTPCode {
		return false
	}
	if r.EnhancedCode != "" && r.EnhancedCode != sig.EnhancedCode {
		return false
	}
	if r.Provider != "" && r.Provider != sig.Provider {
		return false
	}
	if r.Pattern != "" && !strings.Contains(strings.ToLower(sig.Diagnostic), strings.ToLower(r.Pattern)) {
		return false
	}
	return true
}

// specificity scores how specific a rule is, so that among rules of equal
// priority the one constraining more fields wins.
func (r *BounceActionRule) specificity() int {
	n := 0
	for _, f := range []string{r.SMTPCode, r.EnhancedCode, r.Provider, r.Pattern} {
		if f != "" {
			n++
		}
	}
	return n
}

// MatchBounceRule returns the highest-priority active rule matching the bounce,
// or nil when none match. Rules are compared by Priority, then specificity.
func MatchBounceRule(rules []*BounceActionRule, sig BounceSignature) *BounceActionRule {
	sig = sig.normalize()
	var best *BounceActionRule
	for _, r := range rules {
		if !r.matches(sig) {
			continue
		}
		if best == nil ||
			r.Priority > best.Priority ||
			(r.Priority == best.Priority && r.specificity() > best.specificity()) {
			best = r
		}
	}
	return best
}

// ValidateBounceRule normalizes and checks a rule before persistence.
func ValidateBounceRule(r *BounceActionRule) error {
	r.SMTPCode = strings.TrimSpace(r.SMTPCode)
	r.EnhancedCode = strings.TrimSpace(r.EnhancedCode)
	r.Provider = strings.ToLower(strings.TrimSpace(r.Provider))
	r.Pattern = strings.TrimSpace(r.Pattern)
	r.Category = strings.TrimSpace(r.Category)
	r.ActionConfig = strings.TrimSpace(r.ActionConfig)
	r.SuggestedAction = strings.TrimSpace(r.SuggestedAction)
	if r.Class == "" {
		// Default the class from the SMTP code (5xx = hard, else soft).
		if strings.HasPrefix(r.SMTPCode, "5") {
			r.Class = BounceClassHard
		} else {
			r.Class = BounceClassSoft
		}
	}
	if r.Class != BounceClassSoft && r.Class != BounceClassHard {
		return Invalid("BOUNCE_RULE_CLASS_INVALID", "class must be soft or hard")
	}
	if _, ok := bounceActions[r.Action]; !ok {
		return Invalid("BOUNCE_RULE_ACTION_INVALID", "action %q is not valid", r.Action)
	}
	if r.SMTPCode != "" && !regexp.MustCompile(`^[245]\d\d$`).MatchString(r.SMTPCode) {
		return Invalid("BOUNCE_RULE_SMTP_INVALID", "smtp_code %q must be a 3-digit 4xx/5xx code", r.SMTPCode)
	}
	if r.EnhancedCode != "" && !enhancedCodeRe.MatchString(r.EnhancedCode) {
		return Invalid("BOUNCE_RULE_ENHANCED_INVALID", "enhanced_code %q is not an x.y.z status", r.EnhancedCode)
	}
	if r.SMTPCode == "" && r.EnhancedCode == "" && r.Provider == "" && r.Pattern == "" {
		return Invalid("BOUNCE_RULE_EMPTY", "a rule must constrain at least one of code, enhanced, provider, pattern")
	}
	if r.MinAttempts < 0 {
		return Invalid("BOUNCE_RULE_MIN_ATTEMPTS_INVALID", "min_attempts must be >= 0")
	}
	r.SuppressTTL = strings.TrimSpace(r.SuppressTTL)
	if r.SuppressTTL != "" {
		if _, ok := ParseFlexDuration(r.SuppressTTL); !ok {
			return Invalid("BOUNCE_RULE_SUPPRESS_TTL_INVALID", "suppress_ttl %q is not a valid duration (e.g. 30d, 12h)", r.SuppressTTL)
		}
	}
	if r.Status == "" {
		r.Status = "active"
	}
	return nil
}

// SortBounceRules orders rules for stable display: priority desc, then
// specificity desc, then SMTP code.
func SortBounceRules(rules []*BounceActionRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		a, b := rules[i], rules[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.specificity() != b.specificity() {
			return a.specificity() > b.specificity()
		}
		return a.SMTPCode < b.SMTPCode
	})
}

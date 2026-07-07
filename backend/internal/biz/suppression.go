package biz

import (
	"strings"
	"time"
	"unicode"
)

// SanitizeAddress normalizes a configured email address or domain for safe use
// in exact-match policy comparisons. Beyond trimming and lowercasing, it strips
// control characters, Unicode whitespace, and zero-width / format runes (e.g.
// U+200B ZERO WIDTH SPACE, U+FEFF, U+00AD SOFT HYPHEN) that copy-paste commonly
// injects and that strings.TrimSpace does NOT remove. A single such hidden rune
// in, say, the DMARC report address otherwise renders verbatim into the
// generated KumoMTA policy and silently breaks the reception-hook exact-address
// catcher, while the domain derived from the part after '@' stays clean — so the
// listener relays the mail (RCPT 250) but the report is never captured.
func SanitizeAddress(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		// Strip control characters and Unicode format runes (Cf: U+200B ZERO
		// WIDTH SPACE, U+200C/D, U+2060, U+FEFF, U+00AD …). Regular/visible
		// whitespace is intentionally LEFT IN so existing validation still rejects
		// addresses/domains that contain spaces; only the surrounding whitespace
		// is trimmed below.
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(strings.TrimSpace(b.String()))
}

// Suppression types.
const (
	SuppressEmail  = "email"
	SuppressDomain = "domain"
)

// Suppression status values.
const (
	SuppressActive   = "active"
	SuppressDisabled = "disabled"
	SuppressExpired  = "expired"
)

// SuppressionEntry blocks outbound mail to an email or domain.
type SuppressionEntry struct {
	ID     string
	Type   string
	Value  string
	Reason string
	Source string
	Status string
	// Mailclass is the message class of the event that triggered the suppression
	// (e.g. a bounce/deferral on that class), for operator context. Empty for
	// manual entries or where the class is unknown. Suppression itself is global
	// — it blocks the recipient regardless of class.
	Mailclass string
	// ExpiresAt is when the entry stops blocking (nil = permanent). The live
	// policy lookup enforces this via the Redis key TTL; this mirrors it for the
	// DB list and the DB-side IsSuppressed check.
	ExpiresAt *time.Time
	// CreatedAt is when the entry was first recorded (the suppression date).
	CreatedAt time.Time
}

// Validate normalizes and checks a suppression entry.
func (s *SuppressionEntry) Validate() error {
	s.Type = strings.ToLower(strings.TrimSpace(s.Type))
	s.Value = NormalizeSuppressionValue(s.Type, s.Value)
	if s.Status == "" {
		s.Status = SuppressActive
	}
	if s.Source == "" {
		s.Source = "manual"
	}
	switch s.Type {
	case SuppressEmail:
		if !strings.Contains(s.Value, "@") {
			return Invalid("SUPPRESSION_EMAIL_INVALID", "email value %q is not valid", s.Value)
		}
	case SuppressDomain:
		if s.Value == "" || strings.Contains(s.Value, "@") {
			return Invalid("SUPPRESSION_DOMAIN_INVALID", "domain value %q is not valid", s.Value)
		}
	default:
		return Invalid("SUPPRESSION_TYPE_INVALID", "type %q is not valid", s.Type)
	}
	if s.Value == "" {
		return Invalid("SUPPRESSION_VALUE_REQUIRED", "value is required")
	}
	return nil
}

// NormalizeSuppressionValue lowercases and trims a suppression value, used both
// when storing and when matching so comparisons are consistent.
func NormalizeSuppressionValue(typ, value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// RecipientDomain extracts the domain part of an email address (lowercased).
func RecipientDomain(email string) string {
	email = SanitizeAddress(email)
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return email[i+1:]
	}
	return ""
}

// MatchesSuppression reports whether a recipient email is suppressed by an entry.
// An email entry matches the exact address; a domain entry matches the domain.
func (s *SuppressionEntry) MatchesSuppression(recipient string) bool {
	if s.Status != SuppressActive {
		return false
	}
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	switch s.Type {
	case SuppressEmail:
		return recipient == s.Value
	case SuppressDomain:
		return RecipientDomain(recipient) == s.Value
	default:
		return false
	}
}

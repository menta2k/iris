package biz

import "strings"

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
	email = strings.ToLower(strings.TrimSpace(email))
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

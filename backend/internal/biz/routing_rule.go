package biz

import (
	"regexp"
	"strings"
)

// Routing match types.
const (
	MatchMailclass       = "mailclass"
	MatchRecipientEmail  = "recipient_email"
	MatchRecipientDomain = "recipient_domain"
)

// Routing target types.
const (
	TargetVMTA      = "vmta"
	TargetVMTAGroup = "vmta_group"
)

// Routing rule status values.
const (
	RoutingStatusActive   = "active"
	RoutingStatusDisabled = "disabled"
)

// DefaultMailClassHeader is the header name used for a mailclass match when the
// rule does not specify one.
const DefaultMailClassHeader = "X-Mail-Class"

// headerNameRe matches an RFC 7230 header field-name (token), up to 128 chars.
var headerNameRe = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_` + "`" + `|~-]{1,128}$`)

// RoutingRule routes matching mail to a VMTA or VMTA group. A mailclass match is
// a header NAME + VALUE pair (MatchHeader + MatchValue, e.g. "X-Mail-Class" +
// "bulk"); recipient matches use MatchValue alone. When more than one active
// rule matches a message, the rule with the highest Priority wins.
type RoutingRule struct {
	ID          string
	Name        string
	MatchType   string
	MatchHeader string // header name for mailclass matches; empty otherwise
	MatchValue  string
	Priority    int
	TargetType  string
	TargetID    string
	Status      string
}

// ValidMatchType reports whether t is a known match type.
func ValidMatchType(t string) bool {
	switch t {
	case MatchMailclass, MatchRecipientEmail, MatchRecipientDomain:
		return true
	default:
		return false
	}
}

// ValidTargetType reports whether t is a known target type.
func ValidTargetType(t string) bool {
	return t == TargetVMTA || t == TargetVMTAGroup
}

// Validate checks routing-rule invariants before persistence.
func (r *RoutingRule) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.MatchType = strings.TrimSpace(r.MatchType)
	r.MatchHeader = strings.TrimSpace(r.MatchHeader)
	if r.Status == "" {
		r.Status = RoutingStatusActive
	}
	if r.Priority == 0 {
		r.Priority = 100
	}

	// Recipient matches are case-insensitive (emails/domains are); a mailclass
	// header value is matched verbatim so case-sensitive values still work.
	if r.MatchType == MatchMailclass {
		r.MatchValue = strings.TrimSpace(r.MatchValue)
		if r.MatchHeader == "" {
			r.MatchHeader = DefaultMailClassHeader
		}
	} else {
		r.MatchValue = strings.ToLower(strings.TrimSpace(r.MatchValue))
		r.MatchHeader = "" // not applicable to recipient matches
	}

	if r.Name == "" {
		return Invalid("ROUTING_NAME_REQUIRED", "routing rule name is required")
	}
	if !ValidMatchType(r.MatchType) {
		return Invalid("ROUTING_MATCH_TYPE_INVALID", "match_type %q is not valid", r.MatchType)
	}
	if r.MatchType == MatchMailclass && !headerNameRe.MatchString(r.MatchHeader) {
		return Invalid("ROUTING_MATCH_HEADER_INVALID", "match_header %q is not a valid header name", r.MatchHeader)
	}
	if r.MatchValue == "" {
		return Invalid("ROUTING_MATCH_VALUE_REQUIRED", "match_value is required")
	}
	if !ValidTargetType(r.TargetType) {
		return Invalid("ROUTING_TARGET_TYPE_INVALID", "target_type %q is not valid", r.TargetType)
	}
	if r.TargetID == "" {
		return Invalid("ROUTING_TARGET_REQUIRED", "target_id is required")
	}
	if r.Priority < 1 || r.Priority > 1000 {
		return Invalid("ROUTING_PRIORITY_RANGE", "priority must be between 1 and 1000")
	}
	if r.Status != RoutingStatusActive && r.Status != RoutingStatusDisabled {
		return Invalid("ROUTING_STATUS_INVALID", "status %q is not valid", r.Status)
	}
	return nil
}

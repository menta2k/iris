package biz

import (
	"net"
	"regexp"
	"strings"
)

// Routing match types.
const (
	MatchMailclass       = "mailclass"
	MatchRecipientEmail  = "recipient_email"
	MatchRecipientDomain = "recipient_domain"
	// MatchSenderIP matches the connecting client's IP (MatchValue is an IP or
	// CIDR) and ASSIGNS a mailclass (AssignMailclass) to mail that has no
	// mailclass header. It has no VMTA/group target; delivery then follows the
	// mailclass rule for the assigned class.
	MatchSenderIP = "sender_ip"
	// MatchHeaderVMTA reads a header (MatchHeader, e.g. "X-Kumo-VMTA") whose VALUE
	// names the egress VMTA to route via. It has no fixed target — the target is
	// the header value, honored only when that VMTA exists and the mail is
	// relayable (outbound). The header is stripped after use.
	MatchHeaderVMTA = "header_vmta"
)

// DefaultVMTAHeader is the header name a header_vmta rule reads when none is set.
const DefaultVMTAHeader = "X-Kumo-VMTA"

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

// mailclassNameRe constrains an assignable mailclass name to a sane charset.
var mailclassNameRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// RoutingRule routes matching mail to a VMTA or VMTA group. A mailclass match is
// a header NAME + VALUE pair (MatchHeader + MatchValue, e.g. "X-Mail-Class" +
// "bulk"); recipient matches use MatchValue alone. A sender_ip match uses
// MatchValue as an IP/CIDR and AssignMailclass as the class to apply. When more
// than one active rule matches a message, the rule with the highest Priority
// wins.
type RoutingRule struct {
	ID          string
	Name        string
	MatchType   string
	MatchHeader string // header name for mailclass matches; empty otherwise
	MatchValue  string
	// Conditions is the OR-list of header/value pairs for a mailclass rule: the
	// rule matches when ANY condition matches. MatchHeader/MatchValue mirror the
	// first condition for backward compatibility (and for filtering). Empty for
	// non-mailclass rules.
	Conditions []RoutingMatchCondition
	Priority   int
	TargetType string
	TargetID   string
	// AssignMailclass is the class set by a sender_ip rule; empty otherwise.
	AssignMailclass string
	Status          string
}

// RoutingMatchCondition is one header NAME + VALUE pair of a mailclass rule.
type RoutingMatchCondition struct {
	Header string
	Value  string
}

// routingConditions returns a mailclass rule's OR-conditions, synthesizing a
// single condition from the legacy MatchHeader/MatchValue when Conditions is
// empty (e.g. rows written before multi-condition support). Header defaults to
// DefaultMailClassHeader.
func routingConditions(r *RoutingRule) []RoutingMatchCondition {
	if len(r.Conditions) > 0 {
		return r.Conditions
	}
	header := r.MatchHeader
	if header == "" {
		header = DefaultMailClassHeader
	}
	return []RoutingMatchCondition{{Header: header, Value: r.MatchValue}}
}

// ValidMatchType reports whether t is a known match type.
func ValidMatchType(t string) bool {
	switch t {
	case MatchMailclass, MatchRecipientEmail, MatchRecipientDomain, MatchSenderIP, MatchHeaderVMTA:
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

	r.AssignMailclass = strings.TrimSpace(r.AssignMailclass)

	// Normalize per match type. Recipient/sender_ip values are lowercased
	// (case-insensitive); a mailclass header value is matched verbatim.
	switch r.MatchType {
	case MatchMailclass:
		// Build the OR-condition list from the explicit Conditions, or synthesize
		// a single condition from the legacy header/value fields.
		if len(r.Conditions) == 0 {
			r.Conditions = []RoutingMatchCondition{{Header: r.MatchHeader, Value: r.MatchValue}}
		}
		seen := map[[2]string]bool{}
		norm := make([]RoutingMatchCondition, 0, len(r.Conditions))
		for _, c := range r.Conditions {
			h := strings.TrimSpace(c.Header)
			if h == "" {
				h = DefaultMailClassHeader
			}
			v := strings.TrimSpace(c.Value)
			if v == "" {
				continue // skip blanks so a stray empty row can't break the rule
			}
			if !headerNameRe.MatchString(h) {
				return Invalid("ROUTING_MATCH_HEADER_INVALID", "match_header %q is not a valid header name", h)
			}
			key := [2]string{h, v}
			if seen[key] {
				continue
			}
			seen[key] = true
			norm = append(norm, RoutingMatchCondition{Header: h, Value: v})
		}
		if len(norm) == 0 {
			return Invalid("ROUTING_MATCH_VALUE_REQUIRED", "at least one match header/value is required")
		}
		r.Conditions = norm
		// Mirror the first condition into the legacy fields (compat + filtering).
		r.MatchHeader = norm[0].Header
		r.MatchValue = norm[0].Value
	case MatchHeaderVMTA:
		// The header's VALUE is the dynamic target; no match value is stored.
		r.Conditions = nil
		r.MatchValue = ""
		if r.MatchHeader == "" {
			r.MatchHeader = DefaultVMTAHeader
		}
	default:
		r.Conditions = nil
		r.MatchValue = strings.ToLower(strings.TrimSpace(r.MatchValue))
		r.MatchHeader = "" // not applicable to recipient/sender_ip matches
	}
	// sender_ip and header_vmta rules have no VMTA/group target; clear any stray
	// value so the row stores a null target.
	targetless := r.MatchType == MatchSenderIP || r.MatchType == MatchHeaderVMTA
	if targetless {
		r.TargetType = ""
		r.TargetID = ""
	}
	if r.MatchType != MatchSenderIP {
		r.AssignMailclass = "" // only meaningful for sender_ip rules
	}

	if r.Name == "" {
		return Invalid("ROUTING_NAME_REQUIRED", "routing rule name is required")
	}
	if !ValidMatchType(r.MatchType) {
		return Invalid("ROUTING_MATCH_TYPE_INVALID", "match_type %q is not valid", r.MatchType)
	}
	if (r.MatchType == MatchMailclass || r.MatchType == MatchHeaderVMTA) && !headerNameRe.MatchString(r.MatchHeader) {
		return Invalid("ROUTING_MATCH_HEADER_INVALID", "match_header %q is not a valid header name", r.MatchHeader)
	}
	// header_vmta needs no match value (the header value is the dynamic target).
	if r.MatchType != MatchHeaderVMTA && r.MatchValue == "" {
		return Invalid("ROUTING_MATCH_VALUE_REQUIRED", "match_value is required")
	}

	if r.MatchType == MatchSenderIP {
		if err := validateIPMatch(r.MatchValue); err != nil {
			return err
		}
		if r.AssignMailclass == "" {
			return Invalid("ROUTING_ASSIGN_MAILCLASS_REQUIRED", "assign_mailclass is required for a sender_ip rule")
		}
		if !mailclassNameRe.MatchString(r.AssignMailclass) {
			return Invalid("ROUTING_ASSIGN_MAILCLASS_INVALID", "assign_mailclass %q is not a valid mailclass name", r.AssignMailclass)
		}
	} else if r.MatchType != MatchHeaderVMTA {
		// mailclass / recipient rules route to a fixed VMTA or group; header_vmta
		// resolves its target dynamically from the header value.
		if !ValidTargetType(r.TargetType) {
			return Invalid("ROUTING_TARGET_TYPE_INVALID", "target_type %q is not valid", r.TargetType)
		}
		if r.TargetID == "" {
			return Invalid("ROUTING_TARGET_REQUIRED", "target_id is required")
		}
	}

	if r.Priority < 1 || r.Priority > 1000 {
		return Invalid("ROUTING_PRIORITY_RANGE", "priority must be between 1 and 1000")
	}
	if r.Status != RoutingStatusActive && r.Status != RoutingStatusDisabled {
		return Invalid("ROUTING_STATUS_INVALID", "status %q is not valid", r.Status)
	}
	return nil
}

// validateIPMatch checks that value is a valid IP address or CIDR block.
func validateIPMatch(value string) error {
	if strings.Contains(value, "/") {
		if _, _, err := net.ParseCIDR(value); err != nil {
			return Invalid("ROUTING_MATCH_VALUE_INVALID", "match_value %q is not a valid CIDR block", value)
		}
		return nil
	}
	if net.ParseIP(value) == nil {
		return Invalid("ROUTING_MATCH_VALUE_INVALID", "match_value %q is not a valid IP address", value)
	}
	return nil
}

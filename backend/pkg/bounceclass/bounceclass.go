// Package bounceclass classifies RFC 3463 enhanced status codes into
// coarse bounce categories for the Bounces dashboard.
//
// We deliberately keep the taxonomy small (10–12 buckets) so operators can
// reason about it. Source: RFC 3463 + observed practice from large
// ESPs (SES bounce subtypes, SendGrid drop reasons). Each row maps a
// 3-tuple `(class, subject, detail)` (e.g. 5.1.1) to a stable string.
//
// When status is missing or unparseable, callers should pass an empty
// string — Classify returns CategoryUnknown so the UI shows "—" instead
// of crashing on a malformed DSN.
package bounceclass

import "strings"

// Category is a stable string used as the primary axis on the Bounces
// dashboard. New values are additive; never rename an existing one or you
// will silently break operator dashboards keyed on the string.
type Category string

const (
	CategoryUnknownUser     Category = "unknown_user"
	CategoryMailboxFull     Category = "mailbox_full"
	CategoryMailboxDisabled Category = "mailbox_disabled"
	CategoryPolicyBlock     Category = "policy_block"
	CategoryReputationBlock Category = "reputation_block"
	CategoryAuthFailed      Category = "auth_failed"
	CategoryContentRejected Category = "content_rejected"
	CategoryRoutingFailed   Category = "routing_failed"
	CategoryRelayDenied     Category = "relay_denied"
	CategoryTransientNet    Category = "transient_net"
	CategoryTransientSpam   Category = "transient_spam"
	CategoryTransientOther  Category = "transient_other"
	CategoryHardOther       Category = "hard_other"
	CategoryUnknown         Category = "unknown"
)

// Classify returns the coarse Category for an RFC 3463 status code. Input
// is tolerant: empty, malformed, and missing-component values all fall
// through to CategoryUnknown rather than panic.
//
// The mapping favours specificity: a 5.7.1 (policy reject) is reported
// as CategoryPolicyBlock even though a 5.x.x at the class level is
// already "permanent failure". Operators want to know *why* something
// bounced, not just whether it was hard.
func Classify(status string) Category {
	c, sub, det := parse(status)
	switch c {
	case "":
		return CategoryUnknown
	case "4":
		return classify4(sub, det)
	case "5":
		return classify5(sub, det)
	default:
		return CategoryUnknown
	}
}

func parse(s string) (string, string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", ""
	}
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return "", "", ""
	}
	return parts[0], parts[1], parts[2]
}

// classify5 covers the permanent-failure space (5.X.X). The most common
// codes from the wild are listed explicitly; everything else collapses
// to CategoryHardOther so the dashboard still has a row to count.
func classify5(sub, det string) Category {
	switch sub + "." + det {
	// 5.1.x — addressing
	case "1.1", "1.6": // 5.1.1 mailbox does not exist; 5.1.6 destination mailbox has moved
		return CategoryUnknownUser
	case "1.2", "1.10": // 5.1.2 bad domain (mostly), 5.1.10 NULLMX
		return CategoryRoutingFailed
	case "1.0", "1.3":
		return CategoryUnknownUser
	// 5.2.x — mailbox-state
	case "2.1": // mailbox disabled
		return CategoryMailboxDisabled
	case "2.2": // mailbox full
		return CategoryMailboxFull
	case "2.3", "2.4": // message length / system issues
		return CategoryContentRejected
	// 5.3.x — system / mail system; usually transient infra but reported as permanent
	case "3.0", "3.1", "3.2", "3.3", "3.4", "3.5":
		return CategoryHardOther
	// 5.4.x — network / routing
	case "4.0", "4.1", "4.2", "4.3", "4.4", "4.5", "4.6":
		return CategoryRoutingFailed
	// 5.5.x — protocol
	case "5.0", "5.1", "5.2", "5.3", "5.4", "5.5", "5.6":
		return CategoryHardOther
	// 5.6.x — content / message body
	case "6.0", "6.1", "6.2", "6.3", "6.4", "6.5", "6.6", "6.7", "6.8":
		return CategoryContentRejected
	// 5.7.x — security / policy
	case "7.0":
		return CategoryPolicyBlock
	case "7.1": // delivery not authorised; often blocklist
		return CategoryReputationBlock
	case "7.2", "7.3", "7.4", "7.5", "7.6", "7.7", "7.8", "7.9":
		return CategoryAuthFailed
	case "7.10", "7.11", "7.12", "7.13":
		return CategoryAuthFailed
	case "7.21", "7.22", "7.23", "7.24", "7.25", "7.26", "7.27", "7.28":
		return CategoryAuthFailed
	}
	// Fall back on the subject digit if no detail-level match.
	switch sub {
	case "1":
		return CategoryUnknownUser
	case "2":
		return CategoryMailboxFull
	case "4":
		return CategoryRoutingFailed
	case "6":
		return CategoryContentRejected
	case "7":
		return CategoryPolicyBlock
	}
	return CategoryHardOther
}

// classify4 covers the transient-failure space (4.X.X). Soft bounces are
// only auto-suppressed after a threshold, so the categories are mainly
// for analytics — splitting "spam-rejected-but-might-recover" from
// "destination is just slow" is the most useful axis.
func classify4(sub, det string) Category {
	switch sub + "." + det {
	case "2.2": // mailbox full (transient form)
		return CategoryMailboxFull
	case "7.0", "7.1": // policy / blocklist that the receiver flagged transient
		return CategoryTransientSpam
	}
	switch sub {
	case "4": // network / routing
		return CategoryTransientNet
	case "7": // policy / security
		return CategoryTransientSpam
	}
	return CategoryTransientOther
}

// IsHard returns true for status codes whose class digit is "5" — i.e.
// permanent failures that should drive auto-suppression on first sight.
// Empty / unparseable input returns false (treat as transient until proven
// otherwise so we don't suppress on noise).
func IsHard(status string) bool {
	c, _, _ := parse(status)
	return c == "5"
}

// IsTransient returns true for class-4 codes. Mutually exclusive with
// IsHard: if both are false the status is missing or malformed.
func IsTransient(status string) bool {
	c, _, _ := parse(status)
	return c == "4"
}

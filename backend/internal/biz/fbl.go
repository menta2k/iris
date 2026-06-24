package biz

import (
	"context"
	"strings"
)

// Feedback-loop endpoint status values.
const (
	// FBLAwaitingApproval means the domain is not yet enrolled with the mailbox
	// provider: mail arriving at the feedback address is relayed and forwarded to
	// the forward address (so a human can read the provider's confirmation/
	// validation email) instead of being ARF-parsed.
	FBLAwaitingApproval = "awaiting_approval"
	// FBLApproved means enrollment is complete: the domain enables the built-in
	// ARF parser (log_arf) so kumod emits Feedback log records for the consumer.
	FBLApproved = "approved"
)

// FBLEndpoint is a single feedback-loop enrollment: one mailbox-provider feedback
// address at a domain, plus the workflow status that decides whether inbound mail
// is forwarded for human approval or parsed as an ARF report.
type FBLEndpoint struct {
	ID              string
	Domain          string // listener domain controlling relay / ARF
	FeedbackAddress string // the inbound feedback mailbox (forwarding match key)
	ForwardAddress  string // where mail is forwarded while awaiting approval
	Status          string
}

// FBLRepo is the persistence boundary for feedback-loop endpoints.
type FBLRepo interface {
	ListFBLEndpoints(ctx context.Context, page Page) ([]*FBLEndpoint, error)
	// ListFBLEndpointsForPolicy returns the full set (no paging) for rendering.
	ListFBLEndpointsForPolicy(ctx context.Context) ([]*FBLEndpoint, error)
	CreateFBLEndpoint(ctx context.Context, f *FBLEndpoint) (*FBLEndpoint, error)
	UpdateFBLEndpoint(ctx context.Context, id string, f *FBLEndpoint) (*FBLEndpoint, error)
	DeleteFBLEndpoint(ctx context.Context, id string) error
}

// Validate normalizes and checks a feedback-loop endpoint before persistence.
func (f *FBLEndpoint) Validate() error {
	f.Domain = strings.ToLower(strings.TrimSpace(f.Domain))
	f.FeedbackAddress = strings.ToLower(strings.TrimSpace(f.FeedbackAddress))
	f.ForwardAddress = strings.ToLower(strings.TrimSpace(f.ForwardAddress))
	f.Status = strings.ToLower(strings.TrimSpace(f.Status))
	if f.Status == "" {
		f.Status = FBLAwaitingApproval
	}

	switch f.Status {
	case FBLAwaitingApproval, FBLApproved:
	default:
		return Invalid("FBL_STATUS_INVALID", "status %q must be %s or %s", f.Status, FBLAwaitingApproval, FBLApproved)
	}

	if f.Domain == "" {
		return Invalid("FBL_DOMAIN_REQUIRED", "domain is required")
	}
	if len(f.Domain) > 253 || !dnsNameRe.MatchString(f.Domain) {
		return Invalid("FBL_DOMAIN_INVALID", "domain %q is not a valid DNS name", f.Domain)
	}

	if f.FeedbackAddress == "" {
		return Invalid("FBL_FEEDBACK_ADDRESS_REQUIRED", "feedback_address is required")
	}
	if !isValidEmail(f.FeedbackAddress) {
		return Invalid("FBL_FEEDBACK_ADDRESS_INVALID", "feedback_address %q is not a valid email address", f.FeedbackAddress)
	}
	if RecipientDomain(f.FeedbackAddress) != f.Domain {
		return Invalid("FBL_FEEDBACK_DOMAIN_MISMATCH",
			"feedback_address %q must be at domain %q", f.FeedbackAddress, f.Domain)
	}

	// The forward address is only meaningful while awaiting approval (the
	// approved path parses ARF reports rather than forwarding). Require it then.
	if f.Status == FBLAwaitingApproval {
		if f.ForwardAddress == "" {
			return Invalid("FBL_FORWARD_ADDRESS_REQUIRED",
				"forward_address is required while %s", FBLAwaitingApproval)
		}
		if !isValidEmail(f.ForwardAddress) {
			return Invalid("FBL_FORWARD_ADDRESS_INVALID", "forward_address %q is not a valid email address", f.ForwardAddress)
		}
	}
	return nil
}

// isValidEmail does a permissive structural check: a non-empty local part, a
// single '@', and a domain part that is a valid DNS name.
func isValidEmail(s string) bool {
	i := strings.LastIndexByte(s, '@')
	if i <= 0 || i == len(s)-1 {
		return false
	}
	local, domain := s[:i], s[i+1:]
	if local == "" || strings.ContainsAny(local, " @") {
		return false
	}
	return len(domain) <= 253 && dnsNameRe.MatchString(domain)
}

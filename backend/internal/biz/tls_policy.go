package biz

import "strings"

// TLS policy modes. They map to KumoMTA's egress-path enable_tls values:
//
//	TLSModeRequired         -> "Required"         (STARTTLS + valid certificate)
//	TLSModeRequiredInsecure -> "RequiredInsecure" (STARTTLS, skip cert validation)
//
// Both refuse to deliver in cleartext: if the remote does not advertise
// STARTTLS the delivery fails rather than falling back to an unencrypted send.
const (
	TLSModeRequired         = "required"
	TLSModeRequiredInsecure = "required_insecure"
)

// TLS policy status values.
const (
	TLSPolicyActive   = "active"
	TLSPolicyDisabled = "disabled"
)

// TLSPolicy requires TLS when delivering to a destination domain. It is the
// outbound analogue of Postfix's smtp_tls_policy_maps "encrypt"/"secure": when
// kumod delivers to Domain it must negotiate STARTTLS, otherwise the message is
// rejected (never sent in the clear). Enforcement is per egress path, so the
// match is by destination domain (KumoMTA's get_egress_path_config sees the
// routing domain, not the recipient address).
type TLSPolicy struct {
	ID     string
	Domain string
	Mode   string
	Status string
}

// EnableTLSValue returns the KumoMTA enable_tls string for the policy's mode.
func (p *TLSPolicy) EnableTLSValue() string {
	if p.Mode == TLSModeRequiredInsecure {
		return "RequiredInsecure"
	}
	return "Required"
}

// Validate normalizes and checks a TLS policy before persistence.
func (p *TLSPolicy) Validate() error {
	p.Domain = strings.ToLower(strings.TrimSpace(p.Domain))
	p.Mode = strings.ToLower(strings.TrimSpace(p.Mode))
	if p.Mode == "" {
		p.Mode = TLSModeRequired
	}
	if p.Status == "" {
		p.Status = TLSPolicyActive
	}
	if p.Domain == "" {
		return Invalid("TLS_POLICY_DOMAIN_REQUIRED", "domain is required")
	}
	if len(p.Domain) > 253 || !dnsNameRe.MatchString(p.Domain) {
		return Invalid("TLS_POLICY_DOMAIN_INVALID", "domain %q is not a valid DNS name", p.Domain)
	}
	switch p.Mode {
	case TLSModeRequired, TLSModeRequiredInsecure:
	default:
		return Invalid("TLS_POLICY_MODE_INVALID", "mode %q is not valid", p.Mode)
	}
	switch p.Status {
	case TLSPolicyActive, TLSPolicyDisabled:
	default:
		return Invalid("TLS_POLICY_STATUS_INVALID", "status %q is not valid", p.Status)
	}
	return nil
}

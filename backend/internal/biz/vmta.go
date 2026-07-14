package biz

import (
	"net"
	"strings"
)

// VMTA status values.
const (
	VMTAStatusActive   = "active"
	VMTAStatusDisabled = "disabled"
	VMTAStatusDraining = "draining"
)

// VMTA is a virtual MTA used for outbound sending: a self-contained egress
// identity. As of 3.0.0 it OWNS its source IP and EHLO hostname (they are no
// longer inherited from a listener); a listener may optionally be referenced for
// grouping/inbound association but supplies neither.
type VMTA struct {
	ID   string
	Name string
	// IPAddress is the outbound egress source IP this VMTA sends from; EHLOName is
	// the HELO/EHLO hostname it announces. Both are owned by the VMTA.
	IPAddress string
	EHLOName  string
	// ListenerID optionally associates the VMTA with an inbound listener (e.g. the
	// one that receives this IP's bounces). Empty = unattached. It no longer
	// supplies the IP/EHLO.
	ListenerID string
	// MaxConnections caps simultaneous outbound connections for this source
	// (0 = KumoMTA default / unlimited).
	MaxConnections int
	// TLSMode forces the outbound TLS posture for any delivery sent from this
	// VMTA (egress source): "" = no override; required/required_insecure force
	// STARTTLS; opportunistic_insecure/disabled relax it. A per-domain TLS Policy
	// takes precedence over this when both apply.
	TLSMode string
	Status  string
	Notes   string

	// NodeID optionally binds the VMTA to the cluster node its IP lives on.
	// Empty = the local/legacy co-located node. When the owning node exposes a
	// kumo-proxy, other nodes deliver through it so packets always egress from
	// this VMTA's IP.
	NodeID string

	// ListenerName / NodeName are resolved display names, read-only.
	ListenerName string
	NodeName     string
}

// ValidVMTAStatus reports whether status is a known VMTA status.
func ValidVMTAStatus(status string) bool {
	switch status {
	case VMTAStatusActive, VMTAStatusDisabled, VMTAStatusDraining:
		return true
	default:
		return false
	}
}

// Validate checks VMTA invariants before persistence.
func (v *VMTA) Validate() error {
	v.Name = strings.TrimSpace(v.Name)
	v.ListenerID = strings.TrimSpace(v.ListenerID)
	v.NodeID = strings.TrimSpace(v.NodeID)
	v.IPAddress = strings.TrimSpace(v.IPAddress)
	v.EHLOName = strings.TrimSpace(v.EHLOName)
	if v.Status == "" {
		v.Status = VMTAStatusActive
	}

	if v.Name == "" {
		return Invalid("VMTA_NAME_REQUIRED", "vmta name is required")
	}
	if len(v.Name) > 128 {
		return Invalid("VMTA_NAME_TOO_LONG", "vmta name must be at most 128 characters")
	}
	// The VMTA owns its egress identity: a concrete source IP and a valid EHLO
	// hostname are required. A listener attachment is optional.
	if ip := net.ParseIP(v.IPAddress); ip == nil {
		return Invalid("VMTA_IP_INVALID", "ip_address %q is not a valid IP address", v.IPAddress)
	}
	if v.IPAddress == "0.0.0.0" || v.IPAddress == "::" {
		return Invalid("VMTA_IP_WILDCARD", "ip_address must be a concrete IP (it is the egress source)")
	}
	if v.EHLOName == "" {
		return Invalid("VMTA_EHLO_REQUIRED", "ehlo_name is required")
	}
	if len(v.EHLOName) > 253 || !dnsNameRe.MatchString(v.EHLOName) {
		return Invalid("VMTA_EHLO_INVALID", "ehlo_name %q is not a valid DNS name", v.EHLOName)
	}
	if v.MaxConnections < 0 || v.MaxConnections > 100000 {
		return Invalid("VMTA_MAX_CONNECTIONS_RANGE", "max_connections must be between 0 and 100000")
	}
	v.TLSMode = strings.ToLower(strings.TrimSpace(v.TLSMode))
	switch v.TLSMode {
	case "", TLSModeRequired, TLSModeRequiredInsecure, TLSModeOpportunisticInsecure, TLSModeDisabled:
	default:
		return Invalid("VMTA_TLS_MODE_INVALID", "tls_mode %q is not valid", v.TLSMode)
	}
	if !ValidVMTAStatus(v.Status) {
		return Invalid("VMTA_STATUS_INVALID", "status %q is not valid", v.Status)
	}
	return nil
}

// EnableTLSValue returns the KumoMTA enable_tls string for the VMTA's TLS mode,
// or "" when no per-VMTA override is set.
func (v *VMTA) EnableTLSValue() string {
	return enableTLSValue(v.TLSMode)
}

package biz

import (
	"strings"
)

// VMTA status values.
const (
	VMTAStatusActive   = "active"
	VMTAStatusDisabled = "disabled"
	VMTAStatusDraining = "draining"
)

// VMTA is a virtual MTA used for outbound sending. It attaches to a Listener,
// from which it takes its egress source IP and EHLO hostname; the VMTA itself
// only carries a per-source connection limit and operational state.
type VMTA struct {
	ID         string
	Name       string
	ListenerID string
	// MaxConnections caps simultaneous outbound connections for this source
	// (0 = KumoMTA default / unlimited).
	MaxConnections int
	Status         string
	Notes          string

	// Read-only fields resolved from the attached listener for display and
	// policy rendering; not persisted on the VMTA row.
	ListenerName string
	IPAddress    string
	EHLOName     string
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
	if v.Status == "" {
		v.Status = VMTAStatusActive
	}

	if v.Name == "" {
		return Invalid("VMTA_NAME_REQUIRED", "vmta name is required")
	}
	if len(v.Name) > 128 {
		return Invalid("VMTA_NAME_TOO_LONG", "vmta name must be at most 128 characters")
	}
	if v.ListenerID == "" {
		return Invalid("VMTA_LISTENER_REQUIRED", "a vmta must be attached to a listener")
	}
	if v.MaxConnections < 0 || v.MaxConnections > 100000 {
		return Invalid("VMTA_MAX_CONNECTIONS_RANGE", "max_connections must be between 0 and 100000")
	}
	if !ValidVMTAStatus(v.Status) {
		return Invalid("VMTA_STATUS_INVALID", "status %q is not valid", v.Status)
	}
	return nil
}

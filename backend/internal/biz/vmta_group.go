package biz

import "strings"

// VMTAGroup status values.
const (
	VMTAGroupStatusActive   = "active"
	VMTAGroupStatusDisabled = "disabled"
)

// VMTAGroupMember associates a VMTA with a weight inside a group.
type VMTAGroupMember struct {
	VMTAID string
	Weight int
}

// VMTAGroup groups VMTAs for weighted distribution.
type VMTAGroup struct {
	ID      string
	Name    string
	Status  string
	Members []VMTAGroupMember
}

// Validate checks group invariants: unique members, positive weights, and for
// active groups at least one member with positive total weight.
func (g *VMTAGroup) Validate() error {
	g.Name = strings.TrimSpace(g.Name)
	if g.Status == "" {
		g.Status = VMTAGroupStatusActive
	}
	if g.Name == "" {
		return Invalid("VMTA_GROUP_NAME_REQUIRED", "vmta group name is required")
	}
	if g.Status != VMTAGroupStatusActive && g.Status != VMTAGroupStatusDisabled {
		return Invalid("VMTA_GROUP_STATUS_INVALID", "status %q is not valid", g.Status)
	}

	seen := make(map[string]struct{}, len(g.Members))
	total := 0
	for _, m := range g.Members {
		if m.VMTAID == "" {
			return Invalid("VMTA_GROUP_MEMBER_INVALID", "group member vmta_id is required")
		}
		if _, dup := seen[m.VMTAID]; dup {
			return Invalid("VMTA_GROUP_MEMBER_DUPLICATE", "vmta %q appears more than once in the group", m.VMTAID)
		}
		seen[m.VMTAID] = struct{}{}
		if m.Weight <= 0 {
			return Invalid("VMTA_GROUP_WEIGHT_INVALID", "member weight must be a positive integer")
		}
		total += m.Weight
	}

	if g.Status == VMTAGroupStatusActive {
		if len(g.Members) == 0 {
			return Invalid("VMTA_GROUP_EMPTY", "active group must have at least one member")
		}
		if total <= 0 {
			return Invalid("VMTA_GROUP_WEIGHT_ZERO", "active group must have positive total weight")
		}
	}
	return nil
}

// TotalWeight returns the sum of member weights.
func (g *VMTAGroup) TotalWeight() int {
	total := 0
	for _, m := range g.Members {
		total += m.Weight
	}
	return total
}

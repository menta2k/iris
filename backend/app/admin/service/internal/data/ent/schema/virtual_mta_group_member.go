package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// VirtualMtaGroupMember binds one VirtualMta to one VirtualMtaGroup with a
// weight used for weighted-random selection. (group_id, vmta_id) is unique —
// a VMTA may belong to multiple groups but only once per group.
type VirtualMtaGroupMember struct{ ent.Schema }

func (VirtualMtaGroupMember) Fields() []ent.Field {
	return []ent.Field{
		// weight=0 disables a member without removing it; positive weights
		// participate in the random draw proportionally.
		field.Uint32("weight").Default(1),
		// optional priority lets operators stripe a group into preferred /
		// fallback tiers — lower numbers tried first.
		field.Uint32("priority").Default(0),
		field.Bool("enabled").Default(true),
	}
}

func (VirtualMtaGroupMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group", VirtualMtaGroup.Type).
			Ref("members").
			Unique().
			Required(),
		edge.To("vmta", VirtualMta.Type).
			Unique().
			Required(),
	}
}

func (VirtualMtaGroupMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Edges("group", "vmta").Unique(),
	}
}

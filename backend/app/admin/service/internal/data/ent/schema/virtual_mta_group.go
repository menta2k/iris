package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// VirtualMtaGroup is a named bundle of VirtualMtas used as a routing target.
// Routing rules can target a group by name; the policy renderer emits Lua
// that performs weighted-random selection over the members at delivery time.
type VirtualMtaGroup struct{ ent.Schema }

func (VirtualMtaGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(64).Unique(),
		field.String("description").Optional().MaxLen(512),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (VirtualMtaGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("members", VirtualMtaGroupMember.Type),
	}
}

func (VirtualMtaGroup) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name")}
}

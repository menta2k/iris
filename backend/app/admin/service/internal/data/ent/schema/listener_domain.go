package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ListenerDomain is a domain accepted by a listener — used for relay rules
// and per-domain TLS overrides.
type ListenerDomain struct{ ent.Schema }

func (ListenerDomain) Fields() []ent.Field {
	return []ent.Field{
		field.String("domain").NotEmpty().MaxLen(253),
		field.Bool("relay_allowed").Default(false),
		field.Bool("require_tls").Default(false),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (ListenerDomain) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("listener", ListenerConfig.Type).Ref("domains").Unique(),
	}
}

func (ListenerDomain) Indexes() []ent.Index {
	return []ent.Index{index.Fields("domain")}
}

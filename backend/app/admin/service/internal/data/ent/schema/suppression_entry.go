package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SuppressionEntry blocks delivery to an address or domain.
type SuppressionEntry struct{ ent.Schema }

func (SuppressionEntry) Fields() []ent.Field {
	return []ent.Field{
		field.String("address").NotEmpty().MaxLen(320),
		field.String("scope").NotEmpty().MaxLen(16).Default("address"),
		field.String("reason").NotEmpty().MaxLen(32).Default("manual"),
		field.String("note").Optional().MaxLen(512),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("expires_at").Optional().Nillable(),
	}
}

func (SuppressionEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("address", "scope").Unique(),
		index.Fields("created_at"),
		index.Fields("reason", "created_at"),
		index.Fields("expires_at"),
	}
}

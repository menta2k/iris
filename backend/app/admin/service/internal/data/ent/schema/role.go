package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Role groups permissions assignable to users.
//
// Permissions are stored as a string slice in canonical "<resource>:<action>"
// form to avoid a separate permission table — single-tenant simplicity.
type Role struct{ ent.Schema }

func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").NotEmpty().MaxLen(64).Unique(),
		field.String("name").NotEmpty().MaxLen(128),
		field.String("description").Optional().MaxLen(512),
		// Permissions kept as JSON-array strings; ent's "Strings" field maps to
		// jsonb on Postgres which the UI can index without a join.
		field.Strings("permissions").Optional(),
		field.Bool("system").Default(false),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Role) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("users", User.Type).Ref("roles"),
	}
}

func (Role) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code"),
	}
}

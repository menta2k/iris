package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PolicyHistory records each successful Apply of a generated kumomta policy.
type PolicyHistory struct{ ent.Schema }

func (PolicyHistory) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("sha256").NotEmpty().MaxLen(64).Immutable(),
		field.String("note").Optional().MaxLen(512).Immutable(),
		field.Uint32("actor_user_id").Default(0).Immutable(),
		field.Time("applied_at").Default(time.Now).Immutable(),
		// Compressed Lua source — kept for diff/restore.
		field.Text("lua_source").Optional().Immutable(),
	}
}

func (PolicyHistory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("applied_at"),
		index.Fields("sha256"),
	}
}

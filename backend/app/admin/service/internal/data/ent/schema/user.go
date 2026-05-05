package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// User represents an authenticated operator of the admin UI.
type User struct{ ent.Schema }

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("username").
			NotEmpty().
			MaxLen(64).
			Unique(),
		field.String("email").
			NotEmpty().
			MaxLen(320).
			Unique(),
		field.String("display_name").
			Optional().
			MaxLen(128),
		// bcrypt hash, never plaintext.
		field.String("password_hash").
			NotEmpty().
			Sensitive().
			MaxLen(255),
		field.Bool("active").Default(true),
		// Soft-delete: nil means active, non-nil means deactivated/deleted.
		field.Time("deactivated_at").Optional().Nillable(),
		field.Time("last_login_at").Optional().Nillable(),
		field.String("last_login_ip").Optional().MaxLen(64),
		field.Int("failed_logins").Default(0),
		field.Time("locked_until").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("roles", Role.Type),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("username"),
		index.Fields("email"),
		index.Fields("active", "deactivated_at"),
	}
}

package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DkimIdentity is a (domain, selector) signing identity.
type DkimIdentity struct{ ent.Schema }

func (DkimIdentity) Fields() []ent.Field {
	return []ent.Field{
		field.String("domain").NotEmpty().MaxLen(253),
		field.String("selector").NotEmpty().MaxLen(63),
		field.String("algorithm").NotEmpty().MaxLen(32),
		// Public key PEM. Private key remains on disk only.
		field.Text("public_key_pem"),
		field.String("key_path").NotEmpty().MaxLen(1024),
		field.Bool("active").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (DkimIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("domain", "selector").Unique(),
	}
}

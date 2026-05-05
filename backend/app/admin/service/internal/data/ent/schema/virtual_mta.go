package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// VirtualMta describes an outbound delivery profile.
type VirtualMta struct{ ent.Schema }

func (VirtualMta) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(64).Unique(),
		field.Text("source_ips").Optional(),
		field.String("helo_name").MaxLen(253).Optional(),
		field.Uint32("max_connections").Default(0),
		field.Uint32("max_messages_per_connection").Default(0),
		field.Uint32("connect_timeout").Default(30),
		field.String("provider_profile").MaxLen(64).Default("default"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (VirtualMta) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name")}
}

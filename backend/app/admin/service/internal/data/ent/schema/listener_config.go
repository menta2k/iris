package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ListenerConfig represents one kumomta SMTP listener.
type ListenerConfig struct{ ent.Schema }

func (ListenerConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(64).Unique(),
		field.String("listen_addr").NotEmpty().MaxLen(128),
		field.String("hostname").NotEmpty().MaxLen(253),
		field.Bool("tls_enabled").Default(false),
		field.String("tls_cert_pem_path").Optional().MaxLen(1024),
		field.String("tls_key_pem_path").Optional().MaxLen(1024),
		field.Bool("require_auth").Default(false),
		field.Uint64("max_message_size").Default(0),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (ListenerConfig) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("domains", ListenerDomain.Type),
	}
}

func (ListenerConfig) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name")}
}

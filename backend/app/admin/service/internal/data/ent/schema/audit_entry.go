package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuditEntry is the immutable, append-only audit trail of every mutating
// admin-API call. Stored on a TimescaleDB hypertable keyed by `at`.
type AuditEntry struct{ ent.Schema }

func (AuditEntry) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "audit_entry"},
	}
}

func (AuditEntry) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.Time("at").Default(time.Now).Immutable(),
		field.String("operation").NotEmpty().MaxLen(255).Immutable(),
		field.String("resource_type").MaxLen(64).Immutable(),
		field.String("resource_id").MaxLen(255).Optional().Immutable(),
		field.Uint32("actor_user_id").Default(0).Immutable(),
		field.String("actor_username").MaxLen(64).Optional().Immutable(),
		field.String("client_ip").MaxLen(64).Optional().Immutable(),
		field.String("user_agent").MaxLen(255).Optional().Immutable(),
		field.String("request_id").MaxLen(128).Optional().Immutable(),
		field.Int32("status_code").Default(0).Immutable(),
		field.String("status_message").MaxLen(512).Optional().Immutable(),
		// JSON snapshots — redaction is the producer's responsibility.
		field.Text("request_json").Optional().Immutable(),
		field.Text("response_json").Optional().Immutable(),
		field.Int64("duration_ms").Default(0).Immutable(),
	}
}

func (AuditEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("at"),
		index.Fields("operation", "at"),
		index.Fields("actor_user_id", "at"),
		index.Fields("resource_type", "resource_id"),
	}
}

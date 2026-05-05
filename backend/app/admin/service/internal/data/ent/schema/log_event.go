package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LogEvent is one record from the kumomta log stream. Stored on a
// TimescaleDB hypertable keyed by `at`.
type LogEvent struct{ ent.Schema }

func (LogEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "log_event"},
	}
}

func (LogEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.Time("at").Default(time.Now).Immutable(),
		field.String("event_type").MaxLen(32).Immutable(),
		field.String("queue").Optional().MaxLen(255).Immutable(),
		field.String("sender").Optional().MaxLen(320).Immutable(),
		field.String("recipient").Optional().MaxLen(320).Immutable(),
		field.String("message_id").Optional().MaxLen(255).Immutable(),
		field.Int32("response_code").Default(0).Immutable(),
		field.String("response_text").Optional().MaxLen(512).Immutable(),
		field.String("source_ip").Optional().MaxLen(64).Immutable(),
		field.String("vmta").Optional().MaxLen(64).Immutable(),
		// mail_class carries the X-Kumo-Mail-Class value the message was
		// tagged with at reception. Lets the Logs UI filter without parsing
		// extra_json; index below makes the query cheap.
		field.String("mail_class").Optional().MaxLen(64).Immutable(),
		field.Text("extra_json").Optional().Immutable(),
	}
}

func (LogEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("at"),
		index.Fields("event_type", "at"),
		index.Fields("recipient", "at"),
		index.Fields("sender", "at"),
		index.Fields("mail_class", "at"),
		index.Fields("message_id"),
	}
}

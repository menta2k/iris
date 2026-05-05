package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// FeedbackReport stores parsed ARF feedback (RFC 5965).
type FeedbackReport struct{ ent.Schema }

func (FeedbackReport) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.Time("received_at").Default(time.Now).Immutable(),
		field.String("feedback_type").NotEmpty().MaxLen(32).Immutable(),
		field.String("user_agent").Optional().MaxLen(255).Immutable(),
		field.String("source_ip").Optional().MaxLen(64).Immutable(),
		field.String("original_recipient").Optional().MaxLen(320).Immutable(),
		field.String("original_sender").Optional().MaxLen(320).Immutable(),
		field.String("original_message_id").Optional().MaxLen(255).Immutable(),
		field.String("reporting_mta").Optional().MaxLen(253).Immutable(),
		field.Time("arrival_date").Optional().Nillable().Immutable(),
		field.Text("redacted_body").Optional().Immutable(),
	}
}

func (FeedbackReport) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("received_at"),
		index.Fields("feedback_type", "received_at"),
		index.Fields("original_recipient"),
	}
}

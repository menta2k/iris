package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MetricSnapshot stores a sample of kumomta queue/delivery counters at a
// point in time. Stored on a TimescaleDB hypertable.
type MetricSnapshot struct{ ent.Schema }

func (MetricSnapshot) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "metric_snapshot"},
	}
}

func (MetricSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.Time("at").Default(time.Now).Immutable(),
		field.String("queue").Optional().MaxLen(255).Immutable(),
		field.Uint64("queue_size").Default(0).Immutable(),
		field.Uint64("delivered_total").Default(0).Immutable(),
		field.Uint64("failed_total").Default(0).Immutable(),
		field.Uint64("deferred_total").Default(0).Immutable(),
		field.Float("delivery_rate_per_min").Default(0).Immutable(),
		field.Bool("suspended").Default(false).Immutable(),
	}
}

func (MetricSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("at"),
		index.Fields("queue", "at"),
	}
}

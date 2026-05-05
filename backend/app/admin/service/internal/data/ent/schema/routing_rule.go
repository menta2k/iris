package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RoutingRule is the top-level rule entity.
type RoutingRule struct{ ent.Schema }

func (RoutingRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(128),
		field.Int32("priority").Default(100),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (RoutingRule) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("conditions", RuleCondition.Type),
		edge.To("target", RuleTarget.Type).Unique(),
	}
}

func (RoutingRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("priority"),
		index.Fields("enabled", "priority"),
	}
}

// RuleCondition is a single match-clause used by a routing rule.
type RuleCondition struct{ ent.Schema }

func (RuleCondition) Fields() []ent.Field {
	return []ent.Field{
		// Allow-list enforced at service layer; stored as a constrained string.
		field.String("field").NotEmpty().MaxLen(64),
		field.String("op").NotEmpty().MaxLen(32),
		field.String("value").NotEmpty().MaxLen(1024),
	}
}

func (RuleCondition) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("rule", RoutingRule.Type).Ref("conditions").Unique(),
	}
}

// RuleTarget is the dispatch target for a routing rule.
type RuleTarget struct{ ent.Schema }

func (RuleTarget) Fields() []ent.Field {
	return []ent.Field{
		field.String("kind").NotEmpty().MaxLen(32),
		field.String("ref").Optional().MaxLen(128),
		field.Uint32("reject_code").Default(0),
		field.String("reject_text").Optional().MaxLen(512),
	}
}

func (RuleTarget) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("rule", RoutingRule.Type).Ref("target").Unique(),
	}
}

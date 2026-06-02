package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LoginPolicy is one login-firewall rule. Rules gate who may authenticate
// by IP/CIDR, time-of-day, or country (REGION). A rule applies globally
// when target_id == 0, or to a single user when target_id is that user's
// id. type ∈ {BLACKLIST, WHITELIST}; BLACKLIST denies on match, WHITELIST
// restricts a method to its matching values. See service.LoginFirewall for
// the evaluation semantics.
//
// MAC and DEVICE are kept in the method enum for forward-compatibility but
// are rejected at create time and never evaluated — a web login can't
// observe a client's MAC across the network.
type LoginPolicy struct{ ent.Schema }

func (LoginPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "login_policies"},
	}
}

func (LoginPolicy) Fields() []ent.Field {
	return []ent.Field{
		// 0 => global rule (applies to every login); otherwise the target
		// user id.
		field.Uint32("target_id").Default(0),

		field.Enum("type").Values("BLACKLIST", "WHITELIST"),
		field.Enum("method").Values("IP", "MAC", "REGION", "TIME", "DEVICE"),

		// value carries the CIDR/IP (method=IP) or ISO-3166-1 alpha-2
		// country code (method=REGION). Ignored for method=TIME.
		field.String("value").Optional().MaxLen(512),

		// time_window holds the JSON-encoded service.TimeWindow for
		// method=TIME. Stored as a string (not ent field.JSON) to keep the
		// generated ent package free of a schema-package type import; the
		// repo (un)marshals it. Empty for non-TIME rules.
		field.String("time_window").Optional().MaxLen(1024),

		field.String("reason").Optional().MaxLen(512),
		field.Bool("enabled").Default(true),

		// Audit metadata — acting user IDs, matching the proto's uint32
		// created_by/updated_by/deleted_by. NOTE: this diverges from
		// global_settings, which stores updated_by as a username string;
		// the audit_entry table still records the actor username, so the
		// numeric id here is a secondary cross-reference.
		field.Uint32("created_by").Optional().Default(0),
		field.Uint32("updated_by").Optional().Default(0),
		field.Uint32("deleted_by").Optional().Default(0),

		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		// Soft-delete sentinel: nil = active, non-nil = deleted.
		field.Time("deleted_at").Optional().Nillable(),
	}
}

func (LoginPolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("target_id"),
		index.Fields("method"),
		index.Fields("type"),
		// Serves the enforcement hot path ListApplicable
		// (enabled + non-deleted + global/user).
		index.Fields("enabled", "deleted_at", "target_id"),
	}
}

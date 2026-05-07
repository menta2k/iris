package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DsnEvent is one parsed asynchronous Delivery Status Notification (RFC 3464)
// — a bounce that came back as inbound mail rather than as an SMTP response.
// Synchronous bounces still flow through LogEvent (event_type='Bounce'); this
// table is exclusively the "they accepted then bounced later" case.
//
// Stored on a TimescaleDB hypertable keyed by received_at. We denormalise
// mail_class / tenant / campaign at insert time (looked up from the
// originating LogEvent) so the Bounces UI can filter without a join.
type DsnEvent struct{ ent.Schema }

func (DsnEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "dsn_event"},
	}
}

func (DsnEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.Time("received_at").Default(time.Now).Immutable(),

		// VERP token recovered from the envelope-recipient local-part. Empty
		// when the DSN arrived at a shared bounce mailbox or when the token
		// failed HMAC validation. The HMAC check happens in the consumer;
		// invalid tokens are dropped before they reach this table.
		field.String("verp_token").Optional().MaxLen(128).Immutable(),

		// Soft FK to log_event.message_id. Nullable because correlation can
		// fail (no token, malformed embedded headers, very old mail outside
		// our retention window). Indexed so the Bounces UI can pivot to the
		// original send timeline in one click.
		field.String("message_id_ref").Optional().MaxLen(255).Immutable(),

		// RFC 3464 fields. final_recipient is the address the bounce is
		// about; original_recipient is the pre-alias-expansion form when
		// the original used DSN's ORCPT extension. Most DSNs only carry
		// final_recipient.
		field.String("original_recipient").Optional().MaxLen(320).Immutable(),
		field.String("final_recipient").Optional().MaxLen(320).Immutable(),

		// "failed" / "delayed" / "delivered" / "relayed" / "expanded" —
		// kept as a string so unexpected values from non-conforming MTAs
		// don't get rejected at the DB layer.
		field.String("action").MaxLen(16).Immutable(),

		// RFC 3463 enhanced status, e.g. "5.1.1". status_class is the first
		// digit ("4" or "5") for cheap filter queries that don't want to
		// substring-match on every row.
		field.String("status").Optional().MaxLen(16).Immutable(),
		field.String("status_class").Optional().MaxLen(2).Immutable(),

		// Diagnostic-Code from the DSN — usually the upstream SMTP response
		// text (e.g. "smtp; 550 5.1.1 user unknown"). Truncated on insert.
		field.String("diagnostic_code").Optional().MaxLen(1024).Immutable(),

		// Remote-MTA reported by the bouncing server. Useful for grouping
		// bounces by destination provider.
		field.String("remote_mta").Optional().MaxLen(253).Immutable(),

		// Coarse classification produced by pkg/bounceclass — e.g.
		// "unknown_user", "mailbox_full", "policy_block". Stable taxonomy,
		// used as the primary axis of the Bounces dashboard.
		field.String("category").Optional().MaxLen(64).Immutable(),

		// Denormalised from the originating log_event.Reception row. Empty
		// string when the message wasn't tagged or the lookup failed.
		field.String("mail_class").Optional().MaxLen(64).Immutable(),
		field.String("tenant").Optional().MaxLen(64).Immutable(),
		field.String("campaign").Optional().MaxLen(64).Immutable(),

		// Size of the raw DSN body in bytes; lets the UI show "View raw"
		// only when the body is small enough to render inline.
		field.Int32("raw_size").Default(0).Immutable(),

		// Full parsed payload as JSON so future schema additions don't
		// require a backfill — same pattern as log_event.extra_json.
		field.Text("extra_json").Optional().Immutable(),
	}
}

func (DsnEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("received_at"),
		index.Fields("final_recipient", "received_at"),
		index.Fields("category", "received_at"),
		index.Fields("mail_class", "received_at"),
		index.Fields("status_class", "received_at"),
		index.Fields("message_id_ref"),
	}
}

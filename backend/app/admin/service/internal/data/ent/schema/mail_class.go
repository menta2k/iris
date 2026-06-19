package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MailClass is a header-driven routing shortcut. Each class declares the
// header NAME and VALUE that identify it; when a message arrives over
// HTTP/SMTP carrying that header=value, kumomta routes it to the class's
// configured VMTA or VMTA group. (This replaces the earlier single global
// header whose value matched a class by name.)
//
// The legacy throughput fields (priority, max_per_minute, max_concurrent)
// were removed; that responsibility belongs on VMTAs / VMTA groups. Existing
// columns are left in place by the migrator (WithDropColumn=false) and are
// simply ignored by the ORM after this change.
type MailClass struct{ ent.Schema }

func (MailClass) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(64).Unique(),
		field.String("description").Optional().MaxLen(512),
		field.Bool("enabled").Default(true),
		// header_name / header_value are the (header, value) pair that
		// matches this class at reception. Optional at the column level so
		// the migrator doesn't fail backfilling existing rows; the service
		// layer enforces presence on create/update.
		field.String("header_name").Optional().MaxLen(128),
		field.String("header_value").Optional().MaxLen(256),
		// target_kind is "vmta" or "vmta_group". Optional at the column
		// level so the migrator doesn't fail on existing rows; the service
		// layer enforces presence on create/update.
		field.String("target_kind").Optional().MaxLen(16),
		field.String("target_ref").Optional().MaxLen(64),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (MailClass) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name")}
}

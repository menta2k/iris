package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MailClass is a header-driven routing shortcut. When a message arrives over
// HTTP/SMTP, kumomta inspects the global header (X-Kumo-Mail-Class by
// default) and uses its value to look up a class by name, then routes the
// message to the configured VMTA or VMTA group.
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

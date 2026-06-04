package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MfaCredential is one second-factor enrolled by a user. Three kinds share
// the table; the `secret` column's meaning depends on kind:
//
//   - totp:        AES-GCM-encrypted base32 TOTP secret (see pkg/crypto).
//   - webauthn:    JSON-encoded credential (id, public key, sign count, …).
//   - backup_code: bcrypt hash of a single-use recovery code.
//
// A user "has MFA" iff an active totp or webauthn credential exists. MFA is
// optional and self-service — there is no per-user "required" flag.
type MfaCredential struct{ ent.Schema }

func (MfaCredential) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mfa_credentials"},
	}
}

func (MfaCredential) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("user_id"),
		field.Enum("kind").Values("totp", "webauthn", "backup_code"),
		// Secret material — never returned to clients. Sensitive() keeps it
		// out of ent's default debug logging.
		field.String("secret").Sensitive().MaxLen(4096),
		// Human label for passkeys (e.g. "YubiKey 5"); unused for totp/backup.
		field.String("label").Optional().MaxLen(128),
		field.Enum("status").Values("active", "disabled").Default("active"),
		// WebAuthn signature counter for clone detection.
		field.Uint32("sign_count").Default(0),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		// When enrollment was confirmed (totp/webauthn).
		field.Time("confirmed_at").Optional().Nillable(),
		// When a backup code was consumed (single-use).
		field.Time("used_at").Optional().Nillable(),
	}
}

func (MfaCredential) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "kind", "status"),
		index.Fields("user_id", "status"),
	}
}

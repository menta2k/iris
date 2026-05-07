package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// AcmeAccount is the operator's ACME account — singleton (id=1, enforced
// by SQL CHECK and by service.AcmeService). One iris instance has one
// account; multiple certificates issue from it. The account's private
// key is stored as a PEM blob; treating it like a secret is the
// operator's responsibility (production setups should encrypt the
// PostgreSQL volume or use TDE).
//
// server_url is the ACME directory URL — Let's Encrypt prod
// (`https://acme-v02.api.letsencrypt.org/directory`) or staging
// (`https://acme-staging-v02.api.letsencrypt.org/directory`).
// Operators rotate between them by editing this field; the row stays
// the same logical "account" because lego will re-register if the
// server changes.
type AcmeAccount struct{ ent.Schema }

func (AcmeAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "acme_account"},
	}
}

func (AcmeAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Default(1).Immutable(),
		field.String("email").NotEmpty().MaxLen(320),
		field.String("server_url").NotEmpty().MaxLen(512),
		// registration_json is the lego-returned acme.Resource encoded
		// as JSON (URI, body, terms-of-service URL). We keep it raw so
		// future lego versions that add fields don't require a schema
		// migration.
		field.Text("registration_json").Optional(),
		// private_key_pem is the account-level private key (NOT the cert
		// keys). lego signs every ACME request with it, so a fresh key
		// means a fresh account.
		field.Text("private_key_pem").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

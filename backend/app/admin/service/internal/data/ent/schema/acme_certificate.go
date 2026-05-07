package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AcmeCertificate is one issued (or in-flight) certificate.
//
// alt_names carries SANs as a JSON-encoded list. For a typical iris
// setup that will be the same as `domain` plus its bounce subdomain
// (so a single cert covers both `example.com` and `bounces.example.com`
// for the listener TLS).
//
// challenge_type ∈ {"http-01", "dns-01"} selects the lego flow.
// dns_provider names the entry in the DNS provider registry; ignored
// for HTTP-01 issuance.
//
// status follows a small state machine: pending → issued → renewing →
// issued ; or pending → failed (with `error` populated) on a hard fail.
// The renewer only touches rows in `issued` state and drops them back
// to `renewing` while it works.
type AcmeCertificate struct{ ent.Schema }

func (AcmeCertificate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "acme_certificate"},
	}
}

func (AcmeCertificate) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Default(1).Immutable(),
		field.String("domain").NotEmpty().MaxLen(253),
		// JSON-encoded []string. ent's Strings does this for us.
		field.Strings("alt_names").Optional(),

		field.String("challenge_type").MaxLen(16).Default("http-01"),
		field.String("dns_provider").Optional().MaxLen(64),

		// Issued cert + chain (PEM). cert_pem is the leaf+intermediates
		// concatenated (what kumomta wants in tls_certificate). key_pem
		// is the corresponding private key.
		field.Text("cert_pem").Optional(),
		field.Text("key_pem").Optional(),

		// Path on disk where the issuer mirrored the PEMs for kumomta's
		// reader. Same value the Listener.tls_cert_pem_path / _key_path
		// fields can reference. Empty before first successful issue.
		field.String("cert_pem_path").Optional().MaxLen(1024),
		field.String("key_pem_path").Optional().MaxLen(1024),

		field.Time("expires_at").Optional().Nillable(),
		field.Time("last_renewed_at").Optional().Nillable(),
		field.String("status").MaxLen(16).Default("pending"),
		field.Text("last_error").Optional(),

		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (AcmeCertificate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("domain").Unique(),
		index.Fields("status", "expires_at"),
	}
}

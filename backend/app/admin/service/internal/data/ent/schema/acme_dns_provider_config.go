package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AcmeDnsProviderConfig holds the operator-supplied credentials and
// tunables for one DNS provider used in DNS-01 challenges. The shape is
// intentionally a JSON blob (`config_json`) — provider-specific
// required/optional fields are described by the registry
// (pkg/acmedns) and the UI renders the form dynamically from those
// definitions, so a new provider doesn't need a schema migration.
//
// `provider` is the registry key (cloudflare, route53, …) and is the
// natural primary identity here — operators don't need multiple
// configs for the same provider, so we use it as the unique key.
type AcmeDnsProviderConfig struct{ ent.Schema }

func (AcmeDnsProviderConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "acme_dns_provider_config"},
	}
}

func (AcmeDnsProviderConfig) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Default(1).Immutable(),
		field.String("provider").NotEmpty().MaxLen(64),
		// JSON object: { "dnsApiToken": "…", "dnsTTL": "300", … }.
		// Stored as TEXT (PG JSON would also work but TEXT keeps the
		// migration story simple and ent already serializes maps via
		// Strings/Bytes).
		field.Text("config_json"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.String("updated_by").Optional().MaxLen(64),
	}
}

func (AcmeDnsProviderConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider").Unique(),
	}
}

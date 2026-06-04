package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MailWebhook forwards inbound mail to an HTTP endpoint. When KumoMTA
// receives a message whose recipient matches `address`, the rendered policy
// POSTs the raw RFC822 message to `url`. `address` is either an exact
// recipient (support@kmx.example.com) or a bare domain (support.example.com) for
// a catch-all. Activated on the next policy Apply; the address's domain MX
// must point at this KumoMTA.
type MailWebhook struct{ ent.Schema }

func (MailWebhook) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mail_webhooks"},
	}
}

func (MailWebhook) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(64).Unique(),
		field.String("address").NotEmpty().MaxLen(320),
		field.String("url").NotEmpty().MaxLen(2048),
		// Optional HMAC-SHA256 key; the rendered policy signs the body with
		// it and sends X-Iris-Signature so the endpoint can verify origin.
		field.String("secret").Optional().Sensitive().MaxLen(255),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (MailWebhook) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("address"),
		index.Fields("enabled"),
	}
}

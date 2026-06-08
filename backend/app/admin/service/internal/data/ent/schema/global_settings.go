package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// GlobalSettings is a singleton — id is always 1, enforced by the migration
// (see sql/0006_global_settings.sql) and by service.GlobalSettingsService.
// Operator-tunable global knobs that previously had to live in env vars
// land here so they can be edited from the UI without a redeploy.
//
// Secrets (VERP key, JWT keys, DB DSN, Redis URL) deliberately do NOT live
// here — those stay env-only and use a real secret manager in production.
//
// Empty fields fall back to the env-derived defaults at render time, so an
// untouched DB row preserves the legacy behaviour for existing deploys.
type GlobalSettings struct{ ent.Schema }

func (GlobalSettings) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "global_settings"},
	}
}

func (GlobalSettings) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Default(1).Immutable(),

		// HTTP admin listener bind. "0.0.0.0:8000" in compose;
		// "127.0.0.1:8025" host-native to dodge port collisions with
		// the iris admin service itself.
		field.String("kumo_http_listen").Optional().MaxLen(128),

		// Bind spec for the *default* kumo.start_esmtp_listener block
		// (only consulted when no Listener rows are configured —
		// per-listener entries override). Empty -> "0:2525" fallback.
		field.String("esmtp_listen_addr").Optional().MaxLen(128),

		// CIDRs allowed to relay through the default ESMTP listener
		// (only consulted when no Listener rows are configured —
		// per-listener entries override). Stored as a json-encoded list
		// of strings via ent's value-scanner; we keep it simple by
		// using a Strings field.
		field.Strings("esmtp_relay_hosts").Optional(),

		// CIDRs allowed to call the kumomta admin HTTP API.
		field.Strings("http_trusted_hosts").Optional(),

		// Bounce / DSN configuration. Empty disables the pipeline.
		// bounce_domain (singular) is the legacy single-domain mode;
		// bounce_sender_domains is the multi-domain mode and wins when
		// non-empty.
		field.String("bounce_domain").Optional().MaxLen(253),
		field.Strings("bounce_sender_domains").Optional(),
		field.String("bounce_prefix").Optional().MaxLen(64),

		// Header inspected by the mail-class router. Empty falls back
		// to "X-Kumo-Mail-Class".
		field.String("mail_class_header").Optional().MaxLen(128),

		// Default outbound EHLO/HELO hostname (a FQDN, e.g.
		// "mail.example.com"). Rendered as the egress *path* ehlo_domain so
		// every outbound connection announces a resolvable name instead of
		// the bare system hostname (which trips rspamd HFILTER_HELO_5). A
		// per-VMTA helo_name still overrides this. Also used as the domain
		// for any Message-ID iris generates. Empty = leave kumomta's default
		// (system hostname).
		field.String("egress_ehlo_domain").Optional().MaxLen(253),

		// Outbound retry schedule applied to the normal delivery queue
		// config (make_queue_config). Durations in KumoMTA/Go form
		// ("20m", "4h", "7d"). Empty leaves kumomta's defaults
		// (retry_interval 20m, doubling, max_age 7d).
		//   - retry_interval: base backoff after a TransientFailure.
		//   - max_retry_interval: optional cap on the doubling.
		//   - max_age: give up (permanent bounce) once a message is older.
		field.String("egress_retry_interval").Optional().MaxLen(32),
		field.String("egress_max_retry_interval").Optional().MaxLen(32),
		field.String("egress_max_age").Optional().MaxLen(32),

		// Inbound spam filtering via rspamd. rspamd_mode is "" / "off"
		// (disabled), "tag" (scan + add X-Spam headers, never reject), or
		// "enforce" (honor rspamd's action incl. reject/defer). rspamd_url
		// is the rspamd HTTP endpoint (e.g. http://127.0.0.1:11333). Only
		// mail received for hosted domains is scanned; fail-open if rspamd
		// is unreachable.
		field.String("rspamd_mode").Optional().MaxLen(16),
		field.String("rspamd_url").Optional().MaxLen(512),

		// Iris admin HTTPS termination. When https_listen is set, a
		// kratos transport.Server starts on that bind, terminates TLS
		// using the cert+key paths below, and reverse-proxies to the
		// existing plain HTTP server (default :8000). Both run side by
		// side so internal/healthcheck callers can keep using plain
		// HTTP. Empty disables HTTPS.
		field.String("https_listen").Optional().MaxLen(64),
		field.String("https_cert_pem_path").Optional().MaxLen(1024),
		field.String("https_key_pem_path").Optional().MaxLen(1024),

		// Audit metadata. updated_by carries the operator's username so
		// the audit log can answer "who broke prod" without joining
		// against audit_entry.
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.String("updated_by").Optional().MaxLen(64),
	}
}

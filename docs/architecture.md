# Architecture

Iris is a **control plane** for KumoMTA. It does not relay mail itself; it
configures KumoMTA and observes it. There are two directions of flow:

1. **Configuration (Iris → KumoMTA).** UI/API changes are stored in TimescaleDB.
   When you apply, Iris renders the full active configuration into a single
   KumoMTA Lua policy file, writes it to disk, and reloads (or restarts) `kumod`.
2. **Observation (KumoMTA → Iris).** The generated policy includes a `log_hook`
   that streams KumoMTA's structured log records into Redis. Iris workers consume
   those streams and persist them into TimescaleDB hypertables, which power the
   Logs, Bounces, Feedback, DMARC, and Dashboard views.

## Components

| Component | Role |
| --------- | ---- |
| **Vue SPA** | The admin UI. Talks to the API over HTTP. |
| **Iris API** (Go/Kratos) | gRPC service exposed over an HTTP gateway. Hosts the domain logic and the policy renderer. Default binds: HTTP `:8080`, gRPC `:9090`. |
| **TimescaleDB** (PostgreSQL) | Stores configuration (relational tables) and events (hypertables for mail records, bounces, DMARC, rspamd results, audit). |
| **Redis Streams** | The event bus between KumoMTA and the Iris workers, and for in-process command queues. |
| **KumoMTA** (`kumod`) | The MTA. Loads `iris_generated.lua`. |
| **Workers** | Background goroutines inside the Iris process that consume Redis streams. |

## The generated policy

`internal/biz/kumo_config.go` (and friends) render a `ConfigSnapshot` — the set
of active listeners, VMTAs, groups, routes, DKIM keys, TLS policies, FBL
endpoints, inbound routes, and global settings — into one Lua file. The renderer
emits, among others, these KumoMTA callbacks:

- `kumo.on('init')` — listeners, spool, the log hook, bounce classifier.
- `get_listener_domain` — which inbound domains to accept/relay (bounce, FBL,
  DMARC, inbound-route domains).
- `smtp_server_message_received` — the reception hook: DSN/DMARC/FBL capture,
  inbound-route dispatch, optional rspamd scan, suppression check, VERP rewrite,
  DKIM signing, mailclass classification, egress-pool selection.
- `get_queue_config` / `get_egress_pool` / `get_egress_path_config` — egress
  routing, pool/source selection, connection limits and require-TLS.

> **`init.lua` is a symlink** to `iris_generated.lua`. Changes inside
> `kumo.on('init')` (listeners, spool, log hook) are only picked up by a
> **restart**; everything else can be hot **reloaded**. See
> [KumoMTA config generation](kumomta-config.md).

## The Redis event bus

The generated policy XADDs structured records onto Redis streams; Iris workers
consume them with consumer groups.

| Stream | Producer | Consumer (worker) | Purpose |
| ------ | -------- | ----------------- | ------- |
| `iris.mail.events` | policy `log_hook` | `log-stream` | All KumoMTA log records → `mail_records` |
| `iris.dsn.events` | policy DSN catcher | `dsn` | Inbound bounces (DSNs) → classification + suppression |
| `iris.dmarc.events` | policy DMARC catcher | `dmarc` | Inbound DMARC aggregate reports → parsed reports |
| `iris.rspamd.results` | policy rspamd scan | `rspamd-ingest` | Spam verdicts → `rspamd_filter_results` |
| `iris.queue.commands` | API | `service-control` | Queue suspend/resume/bounce commands |
| `iris.service.commands` | API | `service-control` | Service reload/restart requests |

## Workers

All workers run inside the single Iris process and are supervised (restarted on
panic). They are registered in `cmd/iris/main.go`:

| Worker | Responsibility |
| ------ | -------------- |
| `log-stream` | Ingest KumoMTA logs into `mail_records`; auto-suppress hard bounces; FBL provenance verification |
| `dsn` | Parse async bounce (DSN) messages captured at the bounce domain |
| `dmarc` | Parse DMARC aggregate reports (incl. zip/gzip attachments) |
| `rspamd-ingest` | Persist rspamd verdicts |
| `service-control` | Execute queue and service-control commands against `kumod` |
| `acme-challenge` | Serve ACME HTTP-01 challenges (optional bind) |
| `acme-renewer` | Renew issued certificates on a schedule |
| `errlog-flush` | Flush the generic worker-error log sink |

## Data model

- **Configuration tables** (e.g. `listeners`, `vmtas`, `vmta_groups`,
  `routing_rules`, `dkim_domains`, `inbound_routes`, `global_settings`) hold the
  desired state the renderer reads from.
- **Event hypertables** (TimescaleDB) hold time-series: `mail_records`,
  `bounce_records`, `dmarc_*`, `rspamd_filter_results`, `audit_entries`. Some use
  continuous aggregates for the dashboard.
- **The suppression list lives in Redis** (a write-through cache with per-entry
  TTL) so the policy can consult it at SMTP time; the database is the durable
  mirror.

Migrations are plain SQL in `internal/data/migrations/`, embedded into the
binary and applied on startup when `data.database.migrate_on_start` is true.

## Related

- [Configuration](configuration.md)
- [KumoMTA config generation](kumomta-config.md)
- [Mail logs](mail-logs.md)

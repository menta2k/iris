# Configuration

Iris configuration has two layers:

1. **Process config** — a YAML file (plus environment overrides for secrets) that
   sets transport ports, storage connections, auth, and the KumoMTA/Rspamd
   integration. Read once at startup.
2. **Global settings** — deployment-level policy knobs edited in the UI
   (Settings → Global Settings) and stored in the database. These feed the
   generated KumoMTA policy and several workers, and take effect on the next
   config apply (or immediately, for runtime knobs).

## Process config (YAML)

Pass the file with `-config`:

```sh
go run ./cmd/iris -config configs/iris.example.yaml
```

A missing file falls back to built-in defaults. Structure (defaults shown):

```yaml
server:
  http: { addr: ":8080", timeout: 30s }
  grpc: { addr: ":9090", timeout: 30s }

data:
  database:
    dsn: "postgres://iris:iris@localhost:5432/iris?sslmode=disable"
    max_conns: 10
    min_conns: 2
    conn_max_lifetime: 1h
    migrate_on_start: true        # apply embedded SQL migrations at startup
  redis:
    addr: "localhost:6379"
    password: ""
    db: 0
    consumer_name: "iris-1"       # Redis Streams consumer identity

auth:
  session_ttl: 12h
  session_token_secret: ""        # required unless dev_bypass; see authentication.md
  mfa_required: true
  dev_bypass: false               # true disables auth entirely (local dev only)

kumomta:
  base_url: "http://localhost:8000"
  stub: true                      # true = in-memory MTA stub (no live kumod)
  config_path: "/opt/kumomta/etc/policy/iris_generated.lua"
  reload_command: ""              # e.g. "kcli reload" — reload after a config write
  reload_url: ""                  # HTTP alternative (bump-config-epoch)
  restart_command: ""             # e.g. "systemctl restart kumomta" — for init changes
  restart_url: ""
  log_stream_redis_url: ""        # Redis URL embedded in the policy log_hook; empty derives from redis.addr

rspamd:
  base_url: "http://localhost:11334"
  stub: true
  mode: ""                        # "" / "off" | "tag" | "enforce" (see rspamd.md)

log:
  level: "info"
```

### Stub mode

`kumomta.stub` and `rspamd.stub` default to `true`, so the backend runs with no
live MTA or scanner — config "applies" to an in-memory stub. Set them to `false`
in production and provide the reload/restart hooks.

### Reload vs restart hooks

A config apply that touches the `kumo.on('init')` block (listeners, spool, log
hook) needs a **restart**; anything else can be **reloaded**. Provide either the
`*_command` (executed locally) or the `*_url` (HTTP) variants. When no restart
hook is configured, Iris falls back to a reload and flags that a manual restart
is required. See [KumoMTA config generation](kumomta-config.md).

## Environment overrides

These environment variables override the YAML (useful for secrets and
containers):

| Variable | Overrides |
| -------- | --------- |
| `IRIS_DATABASE_DSN` | `data.database.dsn` |
| `IRIS_REDIS_ADDR` | `data.redis.addr` |
| `IRIS_REDIS_PASSWORD` | `data.redis.password` |
| `IRIS_HTTP_ADDR` | `server.http.addr` |
| `IRIS_GRPC_ADDR` | `server.grpc.addr` |
| `IRIS_SESSION_SECRET` | `auth.session_token_secret` |
| `IRIS_AUTH_DEV_BYPASS` | `auth.dev_bypass` |
| `IRIS_LOG_LEVEL` | `log.level` |

Other startup-only environment variables:

| Variable | Purpose |
| -------- | ------- |
| `IRIS_BOOTSTRAP_ADMIN_EMAIL` / `IRIS_BOOTSTRAP_ADMIN_PASSWORD` | Seed the first owner account on an empty user table ([authentication](authentication.md)) |
| `IRIS_BOUNCE_CLASSIFIER_FILE` | Path to KumoMTA's bounce-classifier rules (default `/opt/kumomta/share/bounce_classifier/iana.toml`; set empty to disable) |
| `IRIS_ACME_CERT_DIR` | Where issued PEM certificates are mirrored (default `/opt/kumomta/etc/tls`) |
| `IRIS_ACME_HTTP_BIND` | Bind for the HTTP-01 challenge responder (`off` to disable) |
| `IRIS_ACME_RENEW_INTERVAL` / `IRIS_ACME_RENEW_BEFORE` | Renewal cadence and lead time |
| `IRIS_MONITORING_KEY` | AES-GCM passphrase encrypting seed-mailbox passwords; required to store credentials ([inbox monitoring](inbox-monitoring.md)) |
| `IRIS_MONITORING_FROM` | Fallback probe sender when an account has no `from_address` |
| `IRIS_MONITORING_FETCH_TIMEOUT` / `IRIS_MONITORING_FETCH_GIVEUP` | Per-connection IMAP/POP3 timeout and fetch retry window |
| `IRIS_MONITORING_RECONCILE_INTERVAL` / `IRIS_MONITORING_SCHEDULE_INTERVAL` / `IRIS_MONITORING_FETCH_INTERVAL` | Cadence of the reconciler, scheduler, and fetch workers |
| `IRIS_OPENAI_API_KEY` / `IRIS_OPENAI_MODEL` / `IRIS_OPENAI_API_BASE` | Enable and target the LLM for subject classification and inbox-monitoring spam analysis |

## Global settings (UI)

Edited under **Settings → Global Settings** (permission `settings:write`) and
stored in the singleton `global_settings` row. Stored values take precedence over
the process-config defaults at render time. Notable knobs:

| Setting | Effect |
| ------- | ------ |
| `rspamd_mode` / `rspamd_url` | Inbound spam filtering ([rspamd](rspamd.md)) |
| `egress_ehlo_domain` | Default outbound EHLO hostname |
| `log_stream_redis_url` | Redis URL embedded in the policy log hook |
| `esmtp_listen` / `http_listen` | Default policy listener binds |
| `egress_retry_interval` / `egress_max_retry_interval` / `egress_max_age` | Outbound retry schedule |
| `bounce_domain` | Domain that captures async DSNs ([bounce handling](bounce-handling.md)) |
| `auto_suppress_hard_bounces` / `soft_bounce_threshold` | Bounce auto-suppression |
| `suppression_ttl` | Default lifetime for suppression entries ([suppressions](suppressions.md)) |
| `dmarc_report_email` | The `rua=` address that captures DMARC reports ([DMARC](dmarc.md)) |
| `fbl_require_verification` | Gate FBL auto-suppression on proven provenance ([feedback loops](feedback-loops.md)) |
| `inbound_maildir_base_path` | Maildir root for inbound maildir routes ([inbound routing](inbound-routing.md)) |
| `admin_http_addr` / `admin_tls_enabled` / `admin_tls_cert_domain` | Iris's own admin server bind + HTTPS |
| `acme_renew_interval` / `acme_renew_before` | ACME renewal schedule ([ACME](acme.md)) |
| `prometheus_url` | Prometheus base URL for the dashboard ([dashboard](dashboard.md)) |

## Related

- [Architecture](architecture.md)
- [Deployment](deployment.md)
- [Authentication](authentication.md)

# KumoMTA config generation & apply

Everything you configure in Iris (listeners, VMTAs, groups, routing, DKIM, TLS
policies, suppression, inbound routes, FBL endpoints, global settings) is
compiled into a **single KumoMTA Lua policy** and applied to `kumod`. This page
explains that pipeline and the reload-vs-restart rule.

UI: **KumoMTA → Config** (permission `service:control`).

## How config becomes Lua

1. The renderer loads a `ConfigSnapshot` — all active configuration entities plus
   the effective global settings.
2. `internal/biz/kumo_config.go` (and the `kumo_config_*.go` files) render the
   snapshot into one Lua file. The output is deterministic — the same snapshot
   always produces the same bytes — and is lint-checked before it is offered.
3. On **apply**, Iris writes the file to `kumomta.config_path`
   (default `/opt/kumomta/etc/policy/iris_generated.lua`) and reloads/restarts
   `kumod`.

`init.lua` on the KumoMTA host is a **symlink** to `iris_generated.lua`, so the
generated file is the live policy.

## What the policy contains

The rendered policy wires the major KumoMTA callbacks:

- **`kumo.on('init')`** — starts ESMTP listeners, configures the spool, the
  bounce classifier, and the `log_hook` that streams structured logs to Redis.
- **`get_listener_domain`** — accepts/relays the inbound domains Iris owns: the
  bounce domain, FBL domains (with ARF parsing), the DMARC report domain, and
  inbound-route domains.
- **`smtp_server_message_received`** — the reception hook. In order: DSN capture,
  DMARC capture, FBL forward/parse, inbound-route dispatch (with optional rspamd
  scan), suppression check (reject if suppressed), VERP envelope rewrite, DKIM
  signing, mailclass classification, and egress-pool selection.
- **`get_queue_config`** — maps the scheduled queue to its protocol: the normal
  egress pool, the DSN/DMARC/rspamd Redis trackers, the webhook poster, a maildir
  destination, or a forwarding smarthost.
- **`get_egress_pool` / `get_egress_path_config`** — pool/source selection,
  per-VMTA connection limits, and require-TLS enforcement.

## Preview before applying

The Config page renders a **preview** of the policy and a checksum. The renderer
also reports lint issues; an invalid policy is flagged rather than applied. The
preview is read-only and safe to view at any time.

## Reload vs restart

KumoMTA distinguishes two kinds of policy change:

| Change touches… | Required action | Why |
| --------------- | --------------- | --- |
| Anything **outside** `kumo.on('init')` (routing, DKIM, queues, suppression, inbound routes, egress) | **Reload** (config-epoch bump) | Runtime callbacks are re-evaluated live |
| **Inside** `kumo.on('init')` — listeners, spool, the log hook | **Restart** | `kumo.on('init')` runs once at process start; a reload will not re-run it |

Iris detects which is needed by comparing the init-block checksum. It then:

- runs `kumomta.reload_command` / `reload_url` for a reload, or
- runs `kumomta.restart_command` / `restart_url` for an init change.

If a restart is required but no restart hook is configured, the apply **falls
back to a reload and flags that a manual restart is required** — so listener
changes silently not taking effect is a classic gotcha. Configure a restart hook
in production.

## Apply is high-risk and audited

Applying config is serialized through a service-control request, requires the
`service:control` permission and an explicit confirmation, and is recorded in the
[audit log](audit-log.md). The apply result reports the checksum, whether a
restart was needed, and a summary.

## Related

- [Architecture](architecture.md) — the full reception-hook pipeline and streams
- [Service control & queues](queues.md)
- [Configuration](configuration.md) — the reload/restart hook settings

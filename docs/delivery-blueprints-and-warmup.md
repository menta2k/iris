# Delivery Blueprints + IP Warmup — Architecture & Plan

## Goal
Replace iris's hand-rolled `get_egress_path_config` (and the warmup `MBP_BUCKET`
map) with KumoMTA's shaping helper, and express outbound delivery limits as three
layered, per-IP concerns ("defense in depth"):

1. **Blueprints (base)** — operator-managed default limits per provider/MX
   pattern. The fallback for new/unknown IPs. Rendered to `iris-base.toml`.
2. **Daily Warmup Engine** — per-IP daily-cap overrides that ramp over a curve
   with an expanding set of targeted providers. Rendered to `iris-warmup.toml`
   (layered after base; last-wins).
3. **Hourly Adaptive Throttling (TSA)** — reactive back-off below the warmup
   ceiling on deferrals/4xx. KumoMTA Traffic Shaping Automation.

Because shaping files layer last-wins and limits are per-IP via the `sources`
sub-table, the daily (warmup) and hourly (TSA) limits live in different layers
and compose by precedence rather than colliding on a single `max_message_rate`.

## KumoMTA mechanics (verified)
- `shaping:setup_with_automation { extra_files = { ... } }` loads the helper and
  registers `get_egress_path_config`. Files layer in order; later files override
  earlier (`replace_base = true` discards an earlier domain block entirely).
- Files may be local paths or http(s) URLs; `.toml` → TOML else JSON.
- Per-source overrides: `[domain."google.com".sources."203.0.113.10"]`.
- `mx_rollup = true` (default) applies a domain block to its whole `site_name`
  (all of a provider's domains), which is the native MBP grouping.
- Reload: config-epoch bump (deterministic; iris already has this path) or the
  helper's file-watching under `setup_with_automation`.

Refs: docs.kumomta.com/userguide/configuration/trafficshaping/,
/reference/kumo.shaping/load/, /userguide/trafficshaping/rollups/,
github.com/KumoCorp/kumomta assets/policy-extras/shaping.toml.

## Column → shaping key mapping (the Blueprints page)
| Page column | Shaping key |
|---|---|
| Provider group (Gmail/Microsoft/Yahoo) | rollup label (`mx_rollup` site grouping) |
| MX PATTERN (`google.com`) | the domain entry |
| CONN RATE (`5/min`) | `max_connection_rate` |
| DELIVERIES/CONN (`10`) | `max_deliveries_per_connection` |
| CONN LIMIT (DEFAULT) (`3`) | `connection_limit` |
| DAILY CAP (DEFAULT) (`150`) | base `max_message_rate` (`"150/day"`) |
| STATUS / toggle | enabled flag |

## Data model
`delivery_blueprints` — base shaping rules:
```
id uuid, provider text, mx_pattern text, conn_rate text ("N/unit"),
deliveries_per_conn int, conn_limit int, daily_cap int,
status text (active|disabled), created_at, updated_at,
unique (mx_pattern)
```
Seed Defaults imports a built-in provider registry (Gmail/Microsoft/Yahoo/…)
matching KumoMTA's community shaping providers.

`warmup_schedules` (exists) gains the expanding-targets curve: each stage lists
the **targeted providers** with per-provider `{HourlyPeak, DailyVolume}`; a
provider absent from a stage is "not yet targeted". (Decision: non-targeted →
no override / low cap, not a hard suspend, to avoid mail aging.)

## Rendering
- `iris-base.toml` — one block per active blueprint:
  ```toml
  [domain."google.com"]
  mx_rollup = true
  max_connection_rate = "5/min"
  max_deliveries_per_connection = 10
  connection_limit = 3
  max_message_rate = "150/day"
  ```
- `iris-warmup.toml` — per-IP daily override per active/paused schedule:
  ```toml
  [domain."google.com".sources."203.0.113.10"]
  max_message_rate = "5000/day"
  ```
- init: `local shaper = shaping:setup_with_automation { extra_files = {base, warmup} }`
  then `kumo.on('get_egress_path_config', shaper.get_egress_path_config)`.
  Removes iris's custom egress-path callback + `MBP_BUCKET`/`WARMUP_RATE` tables.

## Phasing
- **P1 — Blueprints + base shaping — DONE.** `delivery_blueprints` entity +
  migration + seed registry + repo + usecase + proto/service; the Blueprints page
  (grouped cards, edit/toggle, Seed Defaults, Add Rule); `RenderBaseShaping`
  (`iris-base.toml`).
- **Shaping cutover — DONE (opt-in), kumod-verified.** `get_egress_path_config`
  uses `kumo.shaping.load({iris-base.toml, iris-warmup.toml})` and iris overlays
  timeouts + per-source connection cap + require-TLS; the apply adapter writes the
  sidecar TOMLs. Gated by `IRIS_SHAPING_ENABLE=true` (shaping dir = the policy
  dir); default-off keeps the legacy egress path. Verified on a live `kumod`
  (`--validate`): full policy boots; warming source resolves to its warmup
  override, others to the blueprint base, with blueprint connection settings.
  Two bugs the unit tests missed were caught and fixed by the live check (TOML
  key `[domain."x"]`→`["x"]`; warmup override needs `mx_rollup=false` to merge).
- **P2 — Make shaping the default + retire `MBP_BUCKET`:** after a soak with the
  flag on, default it on and remove the legacy custom egress-path Lua. Then the
  **expanding-targets** curve (per-stage provider targeting).
- **P3 — Adaptive (TSA):** enable automation rules for hourly back-off, layered
  under the warmup ceiling.

## Risks
- Init-block change → one-time KumoMTA restart on first deploy.
- Provider-key/`site_name` mismatch silently drops overrides → verify against
  real MX grouping with a kumod check.
- Migrating existing connection-limit + require-TLS behavior into shaping must be
  parity-tested before cutover.

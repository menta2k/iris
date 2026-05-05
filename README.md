# Iris

> The operator console for [KumoMTA](https://kumomta.com).

Iris was the messenger of the gods — the one who carried the mail. Fitting,
since this project's job is making sure yours actually arrives. The Go
framework underneath is [Kratos](https://go-kratos.dev) (god of strength),
which kicked off the Greek-pantheon naming line. We tried to resist. We
failed.

Mail delivery is mostly waiting, occasionally crying, and very occasionally
typing `kumo.set_meta` into Lua at 3am. **Iris is the UI you reach for
instead of doing all three.**

> **Status:** wired end-to-end. The operator UI, policy generator, log
> stream, suppression index, feedback loop, and audit trail are all
> working. The Vue frontend is embedded into the Go binary, so production
> deploys are a **single static binary** — Docker, systemd, or whatever
> exotic orchestrator you're nursing this week.

---

## What you get

| feature | what it actually means for you |
|---|---|
| **Operator UI** | 16 CRUD pages for everything kumomta needs (listeners, DKIM, VMTAs, mail classes, routing rules, suppressions, …). No more SSH-and-edit-Lua-by-hand. |
| **Policy generator** | UI changes → validated Lua `init.lua` → atomic file write → kumomta hot-reloads on its 10s epoch poll. You click Apply, it goes live. |
| **Live logs with timeline reconstruction** | Click any `message_id` in the Logs page and you see the full story for that one submission: Reception → every retry → final Delivery or Bounce. The 3am pager call gets shorter. |
| **Mail-class header routing** | Set `X-Kumo-Mail-Class: marketing` on a message; Iris turns it into a queue tenant + egress-pool selector. `/v1/queues` shows one row per class so you can spot which kind of mail is backing up. |
| **VMTAs + weighted Groups** | Per-IP egress sources, plus groups with per-member weight + priority. Round-robin warming, dedicated IPs for transactional, the usual. |
| **DKIM** | Generate (Ed25519 / RSA-2048 / RSA-4096) **or import** existing PEM keys. Rotation is one click. |
| **Suppression list at scale** | Redis-backed hot-path lookup with a `kumo.memoize` cache. Adding the 8 millionth entry doesn't slow down policy renders or messages. |
| **Inbound feedback loop (ARF)** | Parses [RFC 5965] complaint reports, auto-suppresses the complainant. Nothing for you to do. |
| **Audit trail** | Every admin operation gets a row: actor, IP, status, duration. Compliance auditors stop bothering you. |
| **Prometheus metrics** | `/metrics` on a dedicated listener (default `127.0.0.1:9090`). Per-event-type counters, processing-time histogram, stream-pending gauge, suppression-index size, kumomta-admin-call latency, build info. |
| **Live dashboard** | `/analytics` page reads from a Prometheus-backed admin API: 6 summary cards (delivery rate, bounce rate, stream backlog, suppressions, policy applies), a 1h/6h/24h/7d event-rate chart, and per-mail-class volume table. Auto-refreshes every 15s. |
| **E2E test harness** | Five aiosmtpd mock receivers (accept / bounce / defer / slow / fbl), a Go loadgen, scenario YAML, automatic teardown. `make test` and you're done. |
| **One static binary** | `go:embed` puts the Vue SPA inside the binary. One process, one port, one container. |

[RFC 5965]: https://datatracker.ietf.org/doc/html/rfc5965

---

## Use cases

Real operator stories the UI is shaped around.

### "Why is mail to Gmail backing up?"

Pager fires at 3am. Open the **Logs** page, paste `gmail.com` into
**Recipient**, see a wall of `TransientFailure` rows. Pick one,
click its `message_id` → the page filters to **just that
submission's timeline**:

```
2026-05-05 02:14:11   Reception          250
2026-05-05 02:14:11   TransientFailure   421   try again later (4.7.0 [TS01])
2026-05-05 02:14:32   TransientFailure   421   try again later (4.7.0 [TS01])
2026-05-05 02:15:14   TransientFailure   421   try again later (4.7.0 [TS01])
2026-05-05 02:17:02   Delivery           250   2.0.0 OK 1714879022 g11-…
```

So Gmail throttled you for ~3 minutes, then your retry succeeded.
Now you know it's a Gmail-side issue, not yours, and you can go
back to sleep.

### "We need a dedicated IP pool for the new transactional class"

In the UI:

1. **VMTAs** — add `mta-tx-1` and `mta-tx-2` with their source IPs.
2. **VMTA Groups** — create `tx-pool`, add both VMTAs with weight 1.
3. **Mail Classes** — create `tx`, target = `tx-pool`.
4. **Policy → Apply.**

Senders now add `X-Kumo-Mail-Class: tx` to outbound mail and traffic
flows through the dedicated pool. `/v1/queues` shows `tx@gmail.com`,
`tx@yahoo.com` etc. as separate rows so you can see if the
transactional class is healthy independent of marketing.

### "The suppression list is now 8 million entries"

Past a few hundred thousand entries, the textbook approach (every
suppression embedded as a Lua table literal) starts hurting:
multi-megabyte `init.lua`, slow renders, every kumod replica eating
memory.

Iris doesn't do that. Suppressions live in PostgreSQL (the source of
truth) AND in two Redis SETs (`kumo:supp:addr`, `kumo:supp:dom`).
The Lua handler does a `SISMEMBER` against Redis with a 15-second
memoize cache — `O(1)` policy render regardless of list size,
~0.1–0.2 ms per message blended overhead, and **fails open** if Redis
hiccups (blocking legit mail because Redis was sad is the worse
failure mode).

A boot-time resync rebuilds the Redis SETs from PG with an atomic
key swap. So a Redis flush, a fresh deploy, or a crash mid-write all
heal themselves on the next admin-service start.

### "The compliance auditor wants 90 days of listener changes"

Open the **Audit Log** page. Filter `resource_type = listener`,
`from = 90 days ago`. Export. Done. Every Create / Update / Delete
has actor, IP, status code, and timing.

### "Quarterly DKIM rotation"

**DKIM** page → click *Rotate* on each domain's identity → kumomta
picks up the new keys on the next epoch poll. The UI shows the new
public key for the DNS update; old key stays around so in-flight
messages signed with it still verify.

---

## Deployment

Pick the shape that hurts least.

### A. Docker Compose — local dev / staging

The fast path. One command, gets you everything plus the kumomta
container itself.

```bash
cd deploy
make up                  # timescaledb, redis, kumomta, admin-service
make ps                  # what's running + ports
make logs s=kumomta      # tail one service
make rebuild s=admin-service   # rebuild + hot-swap one service after edits
make nuke                # full teardown including volumes
```

Browse <http://localhost:8000/> and you're at the operator UI.
The same listener also serves `/v1/*` for direct API calls — same
binary, same port, no separate frontend container.

For frontend HMR while developing: `pnpm dev` from `frontend/` gives
you live-reload on `:5175` proxying to the backend on `:8000`.

| service | Docker port | URL |
|---|---|---|
| admin-service (UI + API) | `8000` | <http://localhost:8000/> |
| admin-service gRPC | `9000` | — |
| kumomta SMTP | `2025` | (host:2025 → container :2525) |
| kumomta admin HTTP | `8025` | internal-only |
| admin-service `/metrics` | `9090` | <http://localhost:9090/metrics> (Prometheus scrape target) |
| Prometheus UI | `9091` | <http://localhost:9091/> |
| TimescaleDB | `5432` | `postgres://iris:iris@localhost:5432/iris` |
| Redis | `6379` | `redis://localhost:6379/0` |

### B. Host-native systemd — production on a single host

When kumomta is installed from its `.deb` / `.rpm` and you want
admin-service running alongside it on the same machine. No Docker.
Build a tarball on a build host, ship it, extract, edit env file,
`systemctl enable`. The single binary keeps things simple.

```bash
# build host
cd deploy
make host-package        # → out/iris-host.tar.gz

# target host
sudo tar -xzf iris-host.tar.gz -C /
sudo $EDITOR /etc/iris/iris.env     # JWT secrets, kumomta paths
sudo systemctl daemon-reload
sudo systemctl enable --now iris
```

Port allocation is tweaked so admin-service and kumomta don't fight
over `:8000`:

| service | bind | who reaches it |
|---|---|---|
| admin-service (UI + API) | `0.0.0.0:8000` | external — put TLS in front |
| admin-service gRPC | `127.0.0.1:9000` | internal |
| kumomta SMTP | `0.0.0.0:25` (or 587) | external |
| kumomta admin HTTP | `127.0.0.1:8025` | admin-service only |
| postgres / redis | `127.0.0.1:5432` / `:6379` | internal |

Step-by-step walkthrough: **[`docs/host-native-deploy.md`](docs/host-native-deploy.md)**
covers prereqs, filesystem layout, the bootstrap `init.lua` kumomta
needs before the first apply, user/group setup, and an example nginx
TLS config.

### C. Just the binary — Kubernetes, Nomad, scratch container, …

The Docker image is a distroless wrapper around a static binary. If
your platform of choice can run a Linux ELF and you can give it
PostgreSQL + Redis URLs, you're done. The same binary the systemd
unit launches will run anywhere:

```bash
# Build the binary anywhere (linux amd64, no CGO)
make host-build          # → deploy/out/iris

# Run it
IRIS_LOGSTREAM_REDIS_URL=redis://10.0.0.5:6379/0 \
IRIS_AUTH_ACCESS_SECRET=$(openssl rand -base64 48) \
IRIS_AUTH_REFRESH_SECRET=$(openssl rand -base64 48) \
./iris --conf /etc/iris/configs
```

Kubernetes? `kubectl create deployment iris --image=iris/admin-service:dev`
and a Service on port 8000. Nomad? Wrap it in a `docker` driver job.
Bare ECS Fargate? Same image works.

### D. Behind a TLS reverse proxy

Iris does HTTP plain-text. Put nginx, Caddy, or Traefik in front
for TLS termination + a real domain name. Single-origin means the
proxy config is one `proxy_pass` — UI and API share the same path
namespace.

```nginx
server {
    listen 443 ssl http2;
    server_name iris.example.com;

    ssl_certificate     /etc/letsencrypt/live/iris.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/iris.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_buffering  off;            # for future SSE / log tail
    }
}
```

Add the proxy's public hostname to `server.cors.origins` in
`/etc/iris/configs/server.yaml`.

### Default credentials

**admin / admin** (seeded by `sql/0003_seed_admin.sql`). Rotate
*before* you put this anywhere a stranger can reach it.

---

## Architecture

```
                ┌───────────────────────────────────────────┐
                │   Vue 3 frontend (Vben Admin v5)          │
                │   embedded in the Go binary via go:embed  │
                └─────────────────────┬─────────────────────┘
                                      │ same-origin (Bearer JWT)
                                      ▼
   ┌──────────────────────────────────────────────────────────────┐
   │           Kratos admin service (Go, single binary)           │
   │                                                              │
   │  pkg/spa  → /, /assets/*  (SPA + fallback to index.html)     │
   │  routes   → /api/v1/* re-dispatched to /v1/*                 │
   │                                                              │
   │  HTTP :8000 ┐                                                │
   │             ├─► [auth → audit → authz] ─► gRPC services      │
   │  gRPC :9000 ┘     │                                          │
   │  /metrics   ┘     ├─► ent + pgx ── TimescaleDB hypertables   │
   │  :9090            │     audit_entry / log_event /            │
   │                   │     feedback_reports                     │
   │                   ├─► kumomta admin client                   │
   │                   │     (queues, suspend, bounce)            │
   │                   ├─► Redis                                  │
   │                   │     • kumo.events  (log stream)          │
   │                   │     • kumo:supp:* (suppression index)    │
   │                   ├─► kumopolicy renderer ─► init.lua        │
   │                   └─► Prometheus (read-only, for /v1/dashboard/*)│
   └──────────────────────────────────────────────────────────────┘
                  ▲                                  │
                  │ scrape /metrics                  │ PromQL
                  │                                  ▼
            ┌─────┴──────────────────────────────────────────┐
            │     Prometheus  (compose: iris-prometheus)     │
            │     (UI on host :9091)                         │
            └────────────────────────────────────────────────┘
                                      │
                                      ▼
                                KumoMTA daemon
                                ├─ smtp :2525 (or :25)
                                ├─ http admin :8000 / :8025
                                └─ Lua handlers (suppression check,
                                   mail-class router, DKIM signer,
                                   log_hook → Redis XADD)
```

The shared filesystem path `kumomta/etc/policy/init.lua` is the
contract between admin-service and kumomta: admin-service writes,
kumomta polls every 10s, reloads on hash change. In Docker that
path is a bind-mount; on host-native it's a real on-disk path with
group-shared permissions.

---

## Configuration

Every knob is an env var on the admin-service binary:

| env | default | purpose |
|---|---|---|
| `IRIS_KUMO_API_ENDPOINT` | `http://kumomta:8000` | kumomta admin HTTP — Docker hostname; set to `http://127.0.0.1:8025` on host-native |
| `IRIS_KUMO_API_TOKEN` | *(empty)* | bearer token for kumomta admin |
| `IRIS_KUMO_HTTP_LISTEN` | `0.0.0.0:8000` | bind spec emitted into `kumo.start_http_listener`; set to `127.0.0.1:8025` host-native to avoid `:8000` collision |
| `IRIS_KUMO_POLICY_DIR` | `/opt/kumomta/etc/policy` | where the rendered `init.lua` is written |
| `IRIS_DKIM_KEYS_DIR` | `/opt/kumomta/etc/dkim` | DKIM PEM key store |
| `IRIS_LOGSTREAM_REDIS_URL` | *(unset → log stream + suppression index disabled)* | one URL backs both the log-stream consumer and the suppression hot-path index |
| `IRIS_LOGSTREAM_NAME` | `kumo.events` | Redis Streams key |
| `IRIS_LOGSTREAM_GROUP` | `kumo-ui-tracker` | consumer group |
| `IRIS_LOGSTREAM_WORKERS` | `4` | parallel consumers |
| `IRIS_MAIL_CLASS_HEADER` | `X-Kumo-Mail-Class` | header inspected for the mail-class router |
| `IRIS_METRICS_LISTEN` | `127.0.0.1:9090` | bind for the Prometheus `/metrics` listener; set to `off` to disable, or to `0.0.0.0:9090` when behind a reverse proxy / Docker port-forward |
| `IRIS_PROMETHEUS_URL` | *(unset → /v1/dashboard/* return 503)* | base URL of a Prometheus instance the admin-service queries on the operator UI's behalf for the `/analytics` dashboard. In compose: `http://prometheus:9090`. |
| `IRIS_AUTH_ACCESS_SECRET` / `_REFRESH_SECRET` | *(refused if placeholder)* | JWT HS512 secrets — the binary refuses to start with the shipped placeholder values |

Database DSN, listener bindings, CORS, and TLS live in the Kratos
layered YAML: `backend/app/admin/service/configs/{server,data,auth,kumo,logger}.yaml`.
Host-native installs ship the same files under
`deploy/systemd/configs/` with hostnames pointed at loopback.

---

## Design notes

### Mail-class queue tenants

`X-Kumo-Mail-Class` drives kumomta's queue *tenant*, so `/v1/queues`
shows one row per class (`tx@gmail.com`, `marketing@gmail.com`,
`default@gmail.com`) instead of collapsing everything into a single
egress pool. The pool gets recovered inside `get_queue_config` via a
parallel `CLASS_TO_POOL` lookup table — same Lua chunk, no extra
round-trips. Messages without a class header that match no routing
rule fall through to a `default` tenant.

Operationally: when one ESP starts deferring, you can see *which
class* of mail is affected without manually grouping rows.

### Suppression at scale

The Redis-indexed approach is what makes a 10M+ list practical:

- **PostgreSQL** stays the source of truth (audit, history, UI).
- **Redis** is the hot-path index — two SETs (`kumo:supp:addr`,
  `kumo:supp:dom`) dual-written on every Create/Delete.
- **kumomta Lua** does `SISMEMBER` with `kumo.memoize` (15s TTL,
  100K capacity) and **fails open** on Redis errors.
- **Boot-time resync** rebuilds Redis from PG via temp-key + atomic
  RENAME — heals after a Redis flush or a fresh deploy.

When to upgrade to a bloom filter (interface is already factored
to allow it): Redis SISMEMBER p99 > 5 ms, sustained > 5K msg/s, or
list crosses ~50M entries.

### Single-binary build

The Vue SPA is embedded into the Go binary at compile time:

1. `pnpm build:antd` produces `frontend/apps/admin/dist/`.
2. Build pipeline copies that into `backend/pkg/spa/dist/`.
3. `go build` pulls it in via `//go:embed all:dist`.

The HTTP listener serves three surfaces: `/v1/*` (API),
`/api/v1/*` (re-dispatched to `/v1/*` so the same SPA bundle works
in dev and prod), and `/` + `/assets/*` (SPA with index.html
fallback for client-side routes). Hashed asset filenames get
`Cache-Control: immutable, max-age=1y`; entry points get
`no-cache` so a redeploy doesn't strand clients.

If `pkg/spa/dist` is empty (developer ran `go build` without the
frontend stage), a placeholder page explains the fix and the API
keeps working.

### Render-time policy validation

The `pkg/kumopolicy` renderer never trusts caller input verbatim.
Every operator-controlled string flows through `MustLuaString`
(bracket-string escaping with a forbidden-byte set) or
`sanitizeComment` (newline + null stripping). The validator runs on
every Render call so a programming error in callers can't bypass it.

A useful side-effect: the rendered `init.lua` is human-readable Lua
you can paste into a ticket, diff in PRs, or hand to kumomta support.

### Metrics

Iris exposes Prometheus metrics on a **separate listener** so they
don't share the public UI port. Default bind is `127.0.0.1:9090`;
override with `IRIS_METRICS_LISTEN`, or set it to `off` to disable.

The most useful series — every kumomta event flows through the log
processor, which is the natural hook point:

| metric | type | labels |
|---|---|---|
| `iris_log_events_total` | counter | `event_type`, `mail_class` |
| `iris_log_events_dropped_total` | counter | `reason` (`parse_error` / `persist_error` / `ack_error` / `deadletter`) |
| `iris_log_event_processing_duration_seconds` | histogram | `event_type` |
| `iris_log_stream_pending` | gauge | (XPENDING from the consumer group) |
| `iris_suppression_entries` | gauge | `scope` (`addr` / `domain`) |
| `iris_suppression_ops_total` | counter | `op`, `result` |
| `iris_policy_apply_total` | counter | `result` |
| `iris_kumomta_request_duration_seconds` | histogram | `endpoint`, `method`, `result` |
| `iris_build_info` | gauge | `version`, `go_version` |

Plus the standard `go_*` and `process_*` runtime metrics.

The Docker compose stack ships with Prometheus already wired up — the
`prometheus` service scrapes `admin-service:9090` every 15s. Browse
<http://localhost:9091/> to query / explore. Config lives at
`deploy/prometheus/prometheus.yml`; reload after edits with
`curl -X POST http://localhost:9091/-/reload` (no container restart).

For your own production scrape:

```yaml
scrape_configs:
  - job_name: iris
    scrape_interval: 15s
    static_configs:
      - targets: ['iris.example.com:9090']
        labels:
          env: production
```

A handful of useful PromQL queries to start from:

```promql
# Event rate per type, per minute
sum by (event_type) (rate(iris_log_events_total[1m])) * 60

# Bounce ratio (last 5 min)
sum(rate(iris_log_events_total{event_type="Bounce"}[5m]))
  / sum(rate(iris_log_events_total{event_type="Reception"}[5m]))

# Log-processor p99 latency
histogram_quantile(0.99,
  sum by (le, event_type) (rate(iris_log_event_processing_duration_seconds_bucket[5m])))

# Backlog alarm: pending entries climbing
iris_log_stream_pending > 1000
```

Disable for environments that already collect metrics via OTel:
`IRIS_METRICS_LISTEN=off`.

### Dashboard API

The `/analytics` page in the SPA doesn't talk to Prometheus directly —
it goes through three curated admin-service endpoints that bake the
PromQL queries server-side:

| endpoint | shape | notes |
|---|---|---|
| `GET /v1/dashboard/summary` | point-in-time totals (24h) — events_24h by type, delivery + bounce rates, stream backlog, suppression entries by scope, policy-apply outcomes, generated_at | one round-trip for the cards |
| `GET /v1/dashboard/event-rates?range=1h&step=30s` | per-event-type series, points = events/sec over a 5-min trailing window | `range` clamped 1m–7d; `step` clamped 15s–30m; max 720 points per series |
| `GET /v1/dashboard/by-class` | volume + delivery rate per mail_class over 24h | unclassified bucket sorts last |

Why server-side rather than letting the SPA query Prometheus directly:

- **Same-origin** — UI on `:8000`, Prometheus on `:9091`. CORS would
  need wiring on both sides, plus we'd be exposing Prometheus to
  whoever can reach the UI.
- **Auth** — the SPA's bearer token is the iris JWT, which Prometheus
  doesn't speak. The backend hop reuses the existing auth chain.
- **PromQL leakage** — operators editing dashboard widgets shouldn't
  need to learn PromQL. The queries live in `service/dashboard.go`
  where they're reviewed and tested; the JSON shapes are tailored to
  the cards.

When `IRIS_PROMETHEUS_URL` is unset, every dashboard endpoint returns
`503 METRICS_NOT_CONFIGURED` and the `/analytics` page degrades to a
"Metrics backend not configured" alert banner — the rest of the
operator UI is unaffected.

---

## Repository layout

```
backend/                  # Go monorepo, single binary
  api/protos/             # protobuf source; buf-driven codegen
  app/admin/service/      # the admin service (incl. DashboardService)
  pkg/                    # kumopolicy / kumomta / suppressionindex /
                          # logstream / fbl / spa / metrics / promquery /
                          # crypto / jwt / …
  scripts/loadgen/        # E2E harness (scenario YAML + asserter)
  sql/                    # bootstrap SQL (hypertables, seeds)
  Dockerfile              # 3-stage build: pnpm → embed → go build

frontend/                 # pnpm + turbo monorepo (Vben Admin v5)
  apps/admin/             # the lone app; views/kumo/ has one folder
                          # per CRUD page

kumomta/etc/              # shared bind-mount with kumomta
  policy/init.lua         # generated; operator-applied
  dkim/                   # generated/imported PEM keys

deploy/
  docker-compose.yaml     # base stack (incl. Prometheus)
  docker-compose.test.yaml # mocks + loadgen
  Makefile                # docker / host-native targets
  systemd/                # host-native install bundle
  prometheus/             # scrape config (mounted into the prom container)
  test/                   # mocks + scenarios

docs/host-native-deploy.md
```

---

## Build & test

Backend:
```bash
cd backend
go test -race -count=1 -cover ./...
```

Coverage targets ≥80 % on every security-relevant package
(`pkg/{crypto,jwt,kumopolicy,kumomta,suppressionindex,fbl}`,
service-layer auth/suppression/policy).

Frontend:
```bash
cd frontend
pnpm install
pnpm --filter @vben/web-antd typecheck
pnpm --filter @vben/web-antd build
```

End-to-end:
```bash
cd deploy && make test
```

Code generation (`buf` + `wire` + ent):

| output | tool | used by |
|---|---|---|
| `backend/api/gen/go/**` | `make api` (`buf generate`) | server |
| `frontend/apps/admin/src/generated/api/**` | `make ts` | typed client |
| `backend/.../ent/**` | `make ent` | data layer |
| `backend/.../wire_gen.go` | `make wire` | DI graph |

`make gen` runs them all.

### CI / release

Two GitHub Actions workflows, in `.github/workflows/`:

| workflow | trigger | what it does |
|---|---|---|
| **`ci.yml`** | every push / PR to `main` | parallel jobs: backend `go vet` + `go test -race -cover` + `go build`; frontend `pnpm typecheck` + `pnpm build:antd`. Cancels in-flight runs on rebase. |
| **`release.yml`** | tag push matching `v*` | builds + pushes Docker image to **GHCR** (`ghcr.io/<owner>/iris`) tagged `vX.Y.Z`, `vX.Y`, `vX`, `latest`; produces `iris-host-vX.Y.Z.tar.gz` for systemd installs; creates a GitHub Release with auto-generated notes + install instructions and the tarball attached. |

To cut a release:

```bash
git tag v1.2.3
git push origin v1.2.3
```

The image carries the version through both compile-time (linker
`-X main.version=…`) and runtime (`IRIS_BUILD_VERSION` env →
`iris_build_info{version="v1.2.3"}` gauge in Prometheus). Operators
can pull a specific tag, or track `:latest` for the bleeding edge.

GHCR auths via the built-in `GITHUB_TOKEN` — no PAT secrets to
configure. The first release run creates the package as private;
flip to public in the GitHub UI if you want world-readable pulls.

---

## Security model

- **Authentication.** HS512 access (15 min) + refresh (7 day). Login
  rate-limited; `bcrypt` cost 12. The seeded `admin/admin` is for
  first-boot setup — change it before exposing the service.
- **Authorization.** RBAC matcher in `pkg/authorizer`; routes carry
  `meta.authority`, the router guard enforces.
- **Audit.** Every admin operation produces an `audit_entry` row
  (actor / IP / status / duration); the writer is async + batched,
  with clean shutdown draining its queue.
- **Lua injection.** Every operator-controlled value emitted into
  Lua flows through `MustLuaString`. Suppression values don't reach
  Lua at all (Redis-backed lookup), so that surface is structurally
  closed.
- **Secrets.** JWT secrets refused if placeholder. DKIM private keys
  stored mode 0640 (group-readable for the kumomta uid only).

---

## Roadmap

- Bloom-filter pre-filter in front of Redis SISMEMBER (only when
  Redis becomes a bottleneck — interface already factored).
- gRPC interceptor mapping `pkg/middleware/audit` failures into
  Kratos error codes.
- SSE / long-poll log tail on the Logs page.
- Configurable retention via the UI (TimescaleDB
  `add_retention_policy`).
- IP warm-up workflows (declarative ramp curves, automated
  daily-volume checks against actuals).

---

## License

Not yet selected. PRs welcome; a sense of humour about message
delivery is encouraged but not required.

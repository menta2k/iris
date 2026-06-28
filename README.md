# Iris

**Iris is an administration UI and control plane for [KumoMTA](https://kumomta.com).**

KumoMTA is a high-performance Message Transfer Agent configured in Lua. Iris puts
a web UI, an API, and a relational data model in front of it: you configure
listeners, egress IPs, routing, DKIM, suppression, inbound delivery, and safety
policy in the UI, and Iris compiles that configuration into a KumoMTA Lua policy
(`iris_generated.lua`), writes it to the MTA, and reloads it. It also ingests
KumoMTA's structured logs so the same UI shows deliveries, bounces, complaints,
and DMARC reports.

```
┌────────────┐    config (gRPC/HTTP)     ┌──────────────┐   renders    ┌───────────────────┐
│  Vue SPA   │ ────────────────────────► │   Iris API   │ ───────────► │ iris_generated.lua │
└────────────┘                           │   (Go/Kratos)│              └─────────┬─────────┘
                                         └──────┬───────┘                        │ writes + reloads
                       TimescaleDB + Redis ◄────┘                                ▼
                                ▲                                          ┌───────────┐
                                │       structured logs (Redis Streams)    │  KumoMTA  │
                                └──────────────────────────────────────────│  (kumod)  │
                                                                           └───────────┘
```

## Highlights

- **Outbound**: listeners (inbound MX / submission), VMTAs (egress IP + EHLO),
  weighted VMTA groups (pools), and priority routing rules (by mailclass header,
  recipient, or sender IP).
- **Deliverability**: DKIM signing, a Redis-backed suppression list with TTLs,
  require-TLS policies, async bounce (DSN) processing with VERP and classification,
  feedback-loop (ARF) ingestion with provenance verification, and DMARC aggregate
  report parsing.
- **Inbound routing**: deliver mail for domains you host to a maildir, a
  forwarding smarthost, or a webhook — with optional per-route rspamd scanning.
- **Operations**: live mail logs, queue control, a generic worker-error log,
  service control (reload/restart), a Prometheus-backed dashboard, ACME
  (Let's Encrypt) certificate management, and diagnostic tools (sender diagnose,
  RBL/DNSBL check).
- **Security**: password login with TOTP MFA, signed stateless sessions,
  fine-grained RBAC, and an audit log.

## Tech stack

| Layer    | Technology |
| -------- | ---------- |
| Backend  | Go, [Kratos](https://go-kratos.dev) (gRPC + HTTP gateway), `buf`-generated API |
| Frontend | Vue 3 + TypeScript + Vite |
| Storage  | TimescaleDB (PostgreSQL) for config + event hypertables; Redis Streams for the event bus |
| MTA      | KumoMTA (`kumod`), driven by a generated Lua policy |

## Quick start (local)

1. **Start storage** (TimescaleDB + Redis):

   ```sh
   docker compose -f deploy/compose/docker-compose.yml up -d
   ```

2. **Run the backend** (migrations run on start; `kumomta.stub`/`rspamd.stub`
   default to `true` so no live MTA is required):

   ```sh
   cd backend
   IRIS_BOOTSTRAP_ADMIN_EMAIL=admin@example.com \
   IRIS_BOOTSTRAP_ADMIN_PASSWORD='a-long-password' \
   go run ./cmd/iris -config configs/iris.example.yaml
   ```

   The API serves HTTP on `:8080` and gRPC on `:9090`.

3. **Run the frontend**:

   ```sh
   cd frontend
   pnpm install && pnpm dev
   ```

See [docs/deployment.md](docs/deployment.md) for production (Docker images and a
Debian/RPM package with a systemd unit).

## Repository layout

| Path        | Contents |
| ----------- | -------- |
| `backend/`  | Go API, business logic, data access, workers, KumoMTA policy renderer |
| `frontend/` | Vue SPA |
| `deploy/`   | Docker Compose (dev storage), Dockerfiles, and `nfpm` packaging |
| `docs/`     | This documentation set |
| `specs/`    | Original feature specification and plan |

Backend internals: `cmd/iris` (entrypoint + wiring), `internal/biz` (domain
logic + the policy renderer), `internal/data` (Postgres/Redis repositories +
SQL migrations), `internal/service` (API handlers), `internal/worker`
(stream consumers), `api/iris/admin/v1` (proto + generated code).

## Documentation

Start with the [documentation index](docs/README.md). Key entry points:

- [Architecture](docs/architecture.md) — how the control plane and event bus fit together
- [Configuration](docs/configuration.md) — config file, environment variables, global settings
- [KumoMTA config generation](docs/kumomta-config.md) — how UI config becomes Lua, and reload-vs-restart
- [Authentication](docs/authentication.md) & [Authorization](docs/authorization.md)

## License

See the repository for license details.

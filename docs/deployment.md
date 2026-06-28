# Deployment

Iris ships three deployment aids under `deploy/`:

| Path | Purpose |
| ---- | ------- |
| `deploy/compose/` | Docker Compose for **local dev storage** (TimescaleDB + Redis) |
| `deploy/docker/` | Dockerfiles for the backend and the frontend (nginx) |
| `deploy/nfpm/` | An `nfpm` recipe that builds a Debian/RPM package with a systemd unit |

## Local development storage

The Compose file starts only the dependencies — TimescaleDB (PostgreSQL 16 +
`timescaledb`) on `5432` and Redis 7 on `6379`:

```sh
docker compose -f deploy/compose/docker-compose.yml up -d     # start
docker compose -f deploy/compose/docker-compose.yml ps        # status
docker compose -f deploy/compose/docker-compose.yml down      # stop (keeps data)
docker compose -f deploy/compose/docker-compose.yml down -v   # stop + wipe data
```

Default connections (match `backend/configs/iris.example.yaml`):

| Service | Connection |
| ------- | ---------- |
| TimescaleDB | `postgres://iris:iris@localhost:5432/iris?sslmode=disable` |
| Redis | `localhost:6379` (no password) |

Then run the backend (`go run ./cmd/iris -config ...`) and frontend
(`pnpm dev`) on the host, as in the [root README](../README.md).

## Container images

`deploy/docker/backend.Dockerfile` builds the Go API; `frontend.Dockerfile`
builds the SPA and serves it with nginx (`frontend.nginx.conf`). In a containerized
stack, remember that **KumoMTA reaches Redis at a different address than the
backend may** — set `kumomta.log_stream_redis_url` (or the
`log_stream_redis_url` global setting) to the address `kumod` uses (e.g.
`redis://redis:6379`).

## System package (Debian/RPM)

`deploy/nfpm/` packages Iris for a host install:

- `nfpm.yaml` — the package recipe.
- `iris.service` — a systemd unit.
- `configs/iris.yaml` — the installed config file.
- `iris.env.example` — environment overrides (secrets) for the unit.
- `scripts/` — `postinstall` / `preremove` / `postremove` hooks.

Build with `nfpm pkg` (see `nfpm`'s docs) and install the resulting `.deb`/`.rpm`.
Provide secrets via the env file referenced by the unit (at minimum a strong
`IRIS_SESSION_SECRET`, plus `IRIS_DATABASE_DSN` and `IRIS_REDIS_ADDR`).

## Production checklist

- [ ] `auth.dev_bypass: false` and a strong `IRIS_SESSION_SECRET` set
      ([authentication](authentication.md)).
- [ ] `kumomta.stub: false` (and `rspamd.stub: false` if scanning) with working
      reload/restart hooks ([KumoMTA config](kumomta-config.md)).
- [ ] `log_stream_redis_url` reachable from `kumod`.
- [ ] First admin seeded, then the bootstrap env vars removed.
- [ ] TLS for the Iris admin UI — either via the built-in ACME-issued cert
      (`admin_tls_enabled`) or a reverse proxy ([ACME](acme.md)).
- [ ] Prometheus URL set if you want dashboard time-series ([dashboard](dashboard.md)).

## Related

- [Configuration](configuration.md)
- [Architecture](architecture.md)

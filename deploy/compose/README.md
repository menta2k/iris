# Local development services

This Compose file starts the storage dependencies the Iris backend needs for
local development:

- **timescaledb** — TimescaleDB (PostgreSQL 16 with the `timescaledb`
  extension) on port `5432`.
- **redis** — Redis 7 (with Streams) on port `6379`.

## Start / stop

```sh
# Start in the background
docker compose -f deploy/compose/docker-compose.yml up -d

# Check health/status
docker compose -f deploy/compose/docker-compose.yml ps

# Tail logs
docker compose -f deploy/compose/docker-compose.yml logs -f

# Stop (keeps data volumes)
docker compose -f deploy/compose/docker-compose.yml down

# Stop and remove data volumes (fresh database next start)
docker compose -f deploy/compose/docker-compose.yml down -v
```

## Connecting

The default credentials match `backend/configs/iris.example.yaml`.

| Service     | Connection                                                  |
| ----------- | ----------------------------------------------------------- |
| TimescaleDB | `postgres://iris:iris@localhost:5432/iris?sslmode=disable`  |
| Redis       | `localhost:6379` (no password)                              |

Corresponding backend environment variables:

```sh
export IRIS_DATABASE_DSN="postgres://iris:iris@localhost:5432/iris?sslmode=disable"
export IRIS_REDIS_ADDR="localhost:6379"
export IRIS_REDIS_PASSWORD=""
```

Quick connectivity checks:

```sh
# Postgres / TimescaleDB
docker exec -it iris-timescaledb psql -U iris -d iris -c "SELECT extname FROM pg_extension;"

# Redis
docker exec -it iris-redis redis-cli ping
```

Once the services report healthy, run the backend (it migrates on start by
default):

```sh
cd backend && go run ./cmd/iris
```

## Traffic Shaping Automation (TSA) daemon — optional

The `tsa-daemon` service (profile `tsa`) provides the adaptive/hourly back-off
that layers **under** the IP-warmup ceiling. It is opt-in and not part of the
default dev stack.

```sh
# Start the TSA daemon
docker compose -f deploy/compose/docker-compose.yml --profile tsa up -d tsa-daemon

# Point the backend at it so the generated kumod policy publishes delivery
# events to it and subscribes for back-off overrides:
export IRIS_TSA_URL="http://localhost:8008"
cd backend && go run ./cmd/iris
```

| Service    | Connection / API                          |
| ---------- | ----------------------------------------- |
| tsa-daemon | `http://localhost:8008` (publish + subscribe) |

The daemon's automation rules come from `deploy/compose/tsa_init.lua` (which
loads KumoMTA's community shaping config for tuned per-provider rules). TSA only
**tightens** rates on bad signals (4xx / deferrals); it never raises them above
the warmup cap. See `docs/ip-warmup.md` and `docs/delivery-blueprints-and-warmup.md`.

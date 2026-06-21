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

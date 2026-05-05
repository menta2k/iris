# Host-native deployment

This guide installs `iris` (the operator console binary) and
`kumomta` as **systemd services on the same host**, sharing loopback
for everything except SMTP. The single-binary build embeds the Vue
SPA, so one process serves both API and UI; no nginx is required for
the UI itself, though you'll likely put one in front for TLS.

For Docker development, use `make up` from `deploy/`. This document is
for production / staging on a Linux host.

---

## Topology

```
              host (loopback only, except SMTP)
   ┌──────────────────────────────────────────────────────────┐
   │                                                          │
   │   iris.service                                  │
   │     ├─ HTTP  0.0.0.0:8000   (UI + /v1 API)               │
   │     └─ gRPC  127.0.0.1:9000                              │
   │                                                          │
   │   kumomta.service                                        │
   │     ├─ SMTP  0.0.0.0:25     (or 587 / 2525)              │
   │     └─ admin 127.0.0.1:8025 (loopback, kumo-ui only)     │
   │                                                          │
   │   postgresql.service   (with TimescaleDB extension)      │
   │     └─ tcp   127.0.0.1:5432                              │
   │                                                          │
   │   redis.service                                          │
   │     └─ tcp   127.0.0.1:6379                              │
   │                                                          │
   └──────────────────────────────────────────────────────────┘
                        │
                        ▼
                  reverse proxy (optional, your choice)
                        │
                        ▼
                       :443 → admin-service:8000
```

The single port collision worth highlighting: kumomta's stock HTTP
admin defaults to `:8000` and so does admin-service. We move kumomta's
admin to `127.0.0.1:8025` via the rendered `kumo.start_http_listener`
block, so admin-service keeps `:8000` for the UI.

---

## Prereqs

| component | install how |
|---|---|
| **PostgreSQL 14+ with TimescaleDB** | follow the [TimescaleDB self-hosted install](https://docs.timescale.com/self-hosted/latest/install/) for your distro |
| **Redis 7+** | `apt install redis-server` / `dnf install redis` |
| **kumomta** | follow [kumomta install docs](https://docs.kumomta.com/userguide/installation/) (the `.deb` / `.rpm` from kumocorp) |
| **Go 1.25+** | only on the build machine — the runtime host needs only the static binary |
| **Node 20+ + pnpm** | only on the build machine — for the SPA bundle |

---

## 1. Database

```bash
sudo -u postgres psql <<'SQL'
CREATE ROLE iris LOGIN PASSWORD 'iris';
CREATE DATABASE iris OWNER iris;
\c iris
CREATE EXTENSION IF NOT EXISTS timescaledb;
SQL
```

The bootstrap SQL in `backend/sql/00*.sql` creates hypertables, seeds
roles, and seeds the admin/admin user. admin-service runs them on
boot (idempotent), so you don't apply them manually.

---

## 2. Redis

Default config is fine. Confirm it binds to loopback only:

```bash
grep -E '^bind' /etc/redis/redis.conf
# bind 127.0.0.1 -::1
```

`iris` uses two key spaces:

| key | purpose |
|---|---|
| `kumo.events` (Stream) | log_hook from kumomta → admin-service consumer |
| `kumo:supp:addr` / `kumo:supp:dom` (Sets) | hot suppression index, dual-written from PG |
| `kumo:supp:meta` (Hash) | last-resync timestamp + entry count |

---

## 3. Build

On a build machine with Node + Go:

```bash
cd frontend
pnpm install --frozen-lockfile
pnpm build:antd
cp -r apps/admin/dist/. ../backend/pkg/spa/dist/

cd ../backend
CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$(date +%Y%m%d-%H%M%S)" \
    -o ./admin-service ./app/admin/service/cmd/server
```

The single binary embeds the SPA and is statically linked. Ship it
to the target host.

---

## 4. Filesystem layout on the target

```
/usr/local/bin/iris                       # the binary
/etc/iris/configs/                           # kratos layered YAML
   ├─ server.yaml
   ├─ data.yaml
   ├─ auth.yaml
   ├─ kumo.yaml
   └─ logger.yaml
/etc/iris/iris.env                  # env file (root:iris 0640)
/var/lib/iris/                               # reserved for future state
/etc/systemd/system/iris.service          # the unit file

# kumomta-side:
/opt/kumomta/etc/policy/init.lua                   # written by admin-service,
                                                   #   read by kumomta
/opt/kumomta/etc/dkim/                             # DKIM PEM keys
/var/spool/kumomta/                                # spool (kumomta-managed)
/var/log/kumomta/                                  # logs
```

Create user + group:

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin iris
sudo usermod -aG kumomta iris      # so admin-service can write into
                                     # /opt/kumomta/etc/policy + /dkim
sudo chown -R kumod:kumomta /opt/kumomta/etc/{policy,dkim}
sudo chmod 2775 /opt/kumomta/etc/{policy,dkim}   # setgid: new files inherit group
```

Install the artefacts:

```bash
sudo install -m 0755 ./iris /usr/local/bin/
sudo mkdir -p /etc/iris/configs /var/lib/iris
sudo cp deploy/systemd/configs/* /etc/iris/configs/
sudo cp deploy/systemd/iris.env.example /etc/iris/iris.env
sudo cp deploy/systemd/iris.service /etc/systemd/system/

sudo chown root:iris /etc/iris/iris.env
sudo chmod 0640 /etc/iris/iris.env
```

Then **edit `/etc/iris/iris.env`**:

- Generate JWT secrets: `openssl rand -base64 48` (twice; one each for
  access and refresh).
- Confirm `IRIS_KUMO_HTTP_LISTEN=127.0.0.1:8025` and
  `IRIS_KUMO_API_ENDPOINT=http://127.0.0.1:8025` agree.

Also edit `/etc/iris/configs/auth.yaml` and replace the two
placeholder `secret:` values, OR remove them so the env-file values
take precedence.

---

## 5. kumomta config

kumomta's bootstrap policy comes from admin-service: when the
operator clicks **Apply** in the Policy editor, init.lua is written
to `/opt/kumomta/etc/policy/init.lua` and kumomta picks it up via its
10s epoch poll. **Before** the first apply, you need a minimal
init.lua to bootstrap kumomta — copy this in:

```lua
-- /opt/kumomta/etc/policy/init.lua  (replaced by admin-service on first Apply)
local kumo = require 'kumo'

kumo.on('init', function()
  kumo.define_spool { name = 'data', path = '/var/spool/kumomta/data' }
  kumo.define_spool { name = 'meta', path = '/var/spool/kumomta/meta' }
  kumo.configure_local_logs { log_dir = '/var/log/kumomta' }

  -- Loopback-only HTTP admin so iris can reach it. The
  -- rendered policy from admin-service replaces this with the same
  -- bind spec (configurable via IRIS_KUMO_HTTP_LISTEN).
  kumo.start_http_listener {
    listen = '127.0.0.1:8025',
    trusted_hosts = { '127.0.0.0/8' },
  }

  kumo.start_esmtp_listener {
    listen = '0:25',
    relay_hosts = { '127.0.0.0/8' },
  }
end)
```

Make sure kumomta runs as user `kumod`, group `kumomta`, with read
access to `/opt/kumomta/etc/policy/` and `/opt/kumomta/etc/dkim/`.

---

## 6. Start the services

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now postgresql redis-server kumomta
sudo systemctl enable --now iris

# Watch the boot:
sudo journalctl -u iris -f
```

You should see:

```
suppression-resync: rebuilt index with 0 entries in Xms
[HTTP] server listening on: [::]:8000
[gRPC] server listening on: [::]:9000
```

Hit it:

```bash
curl http://127.0.0.1:8000/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}'
```

Open <http://your-host:8000/> in a browser and you're at the operator
UI. Login admin / admin (rotate immediately).

---

## 7. Reverse proxy (optional)

A trivial nginx config:

```nginx
server {
    listen 443 ssl http2;
    server_name kumo-ui.example.com;
    ssl_certificate     /etc/letsencrypt/live/kumo-ui.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/kumo-ui.example.com/privkey.pem;

    # The admin-service serves both the UI and the API on one origin,
    # so a single proxy_pass covers everything.
    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        # Drop the buffering for SSE if/when we add live-tail.
        proxy_buffering off;
    }
}
```

Add the proxy's public hostname to `server.cors.origins` in
`/etc/iris/configs/server.yaml`.

---

## Operator workflow

After the service is running:

1. **Login** as admin/admin → rotate password → create operator users.
2. **Apply policy** from the Policy editor — first apply replaces the
   bootstrap init.lua with the renderer's full output (DKIM signers,
   routing rules, suppression Redis hooks, etc.).
3. **kumomta hot-reloads** the new policy within ~10 seconds (epoch
   poll picks up the file mtime change).
4. **Send mail** through `:25`. Logs flow into Redis →
   admin-service → TimescaleDB; visible in the Logs page within
   ~seconds.

---

## Updating

```bash
# On the build machine:
cd frontend && pnpm build:antd
cp -r apps/admin/dist/. ../backend/pkg/spa/dist/
cd ../backend && go build -trimpath -o ./admin-service ./app/admin/service/cmd/server

# Ship to host:
scp admin-service host:/tmp/
ssh host 'sudo install -m 0755 /tmp/admin-service /usr/local/bin/iris \
         && sudo systemctl restart iris'
```

The admin-service is stateless (state lives in PG + Redis), so a
restart is safe — outstanding requests get cancelled cleanly via the
kratos lifecycle and clients reconnect.

---

## Troubleshooting

| symptom | likely cause |
|---|---|
| `502 KUMOMTA_UNREACHABLE` on /v1/queues | kumomta down, OR `IRIS_KUMO_API_ENDPOINT` doesn't match what `start_http_listener` binds to |
| `policy: read active: open …: permission denied` | admin-service user not in the `kumomta` group, or `/opt/kumomta/etc/policy/` not setgid 2775 |
| `suppression-resync: index unhealthy at boot` | Redis down or `IRIS_LOGSTREAM_REDIS_URL` wrong; admin-service still serves the API but the suppression hot-path is bypassed |
| browser shows "frontend not embedded" | binary built without copying `frontend/apps/admin/dist/` into `backend/pkg/spa/dist/` before `go build` |
| `/v1/auth/login` returns 500 with "secret too short" | JWT secrets in env file are still placeholders; generate with `openssl rand -base64 48` |

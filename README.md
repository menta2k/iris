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
| **Operator UI** | 17 CRUD pages for everything kumomta needs (listeners, DKIM, VMTAs, mail classes, routing rules, suppressions, …). No more SSH-and-edit-Lua-by-hand. |
| **Policy generator** | UI changes → validated Lua `init.lua` → atomic file write → kumomta hot-reloads on its 10s epoch poll. You click Apply, it goes live. |
| **Live logs with timeline reconstruction** | Click any `message_id` in the Logs page and you see the full story for that one submission: Reception → every retry → final Delivery or Bounce. The 3am pager call gets shorter. |
| **Mail-class header routing** | Set `X-Kumo-Mail-Class: marketing` on a message; Iris turns it into a queue tenant + egress-pool selector. `/v1/queues` shows one row per class so you can spot which kind of mail is backing up. |
| **VMTAs + weighted Groups** | Per-IP egress sources, plus groups with per-member weight + priority. Round-robin warming, dedicated IPs for transactional, the usual. |
| **DKIM** | Generate (Ed25519 / RSA-1024 / RSA-2048 / RSA-4096) **or import** existing PEM keys. Rotation is one click. (RSA-1024 is weaker but fits a single DNS TXT string; RSA-2048 is the safer default.) |
| **Suppression list at scale** | Redis-backed hot-path lookup with a `kumo.memoize` cache. Adding the 8 millionth entry doesn't slow down policy renders or messages. |
| **Inbound feedback loop (ARF)** | Parses [RFC 5965] complaint reports, auto-suppresses the complainant. Nothing for you to do. |
| **Async bounce handling (DSN)** | Inbound [RFC 3464] DSNs are accepted at one or more configurable bounce subdomains (multi-domain mode handles cross-org senders), parsed, classified into 14 stable categories, and correlated to the originating send via VERP. Hard bounces auto-suppress; soft bounces suppress after a configurable threshold. Browse them at `/observability/dsns`. |
| **Audit trail** | Every admin operation gets a row: actor, IP, status, duration. Compliance auditors stop bothering you. |
| **MFA (second factor)** | Optional, self-service multi-factor auth: authenticator-app **TOTP**, **WebAuthn passkeys/security keys**, and single-use **backup codes**. Two-step login (password → factor → tokens); TOTP secrets AES-GCM encrypted at rest. Manage your own at `/security/mfa`; admins can reset a locked-out user's MFA from the Users page. |
| **Login firewall** | Gate who can authenticate by IP/CIDR, country (GeoIP), or time-of-day window — global or per-user, blacklist or whitelist. Evaluated before the password check so blocked sources can't even probe credentials. Fails open on indeterminate attributes and guards against locking yourself out. The country database (DB-IP, MaxMind-compatible) auto-downloads and self-updates monthly — no manual provisioning. Manage it at `/security/login-firewall`. |
| **Prometheus metrics** | `/metrics` on a dedicated listener (default `127.0.0.1:9090`). Per-event-type counters, processing-time histogram, stream-pending gauge, suppression-index size, kumomta-admin-call latency, build info. |
| **Live dashboard** | `/analytics` page reads from a Prometheus-backed admin API: 6 summary cards (delivery rate, bounce rate, stream backlog, suppressions, policy applies), a 1h/6h/24h/7d event-rate chart, and per-mail-class volume table. Auto-refreshes every 15s. |
| **E2E test harness** | Five aiosmtpd mock receivers (accept / bounce / defer / slow / fbl), a Go loadgen, scenario YAML, automatic teardown. `make test` and you're done. |
| **One static binary** | `go:embed` puts the Vue SPA inside the binary. One process, one port, one container. |

[RFC 5965]: https://datatracker.ietf.org/doc/html/rfc5965
[RFC 3464]: https://datatracker.ietf.org/doc/html/rfc3464

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

### "Are we sending to dead addresses?"

Open the **Bounces** page (`/observability/dsns`). Filter `category =
unknown_user`, `time = last 7 days`. You're looking at the recipients
that bounced with a 5.1.1 — mailbox doesn't exist. Each one's already
been auto-added to the suppression list, so you're not bouncing them
again; the question is whether your acquisition pipeline is producing
junk addresses.

Click the `message_id` on any row to pivot to **Logs** filtered to that
single submission's timeline (Reception → final Bounce). Click *Details*
for the raw RFC 3464 fields and the bounce sender's diagnostic.

The same workflow with `category = reputation_block` tells you when
some receiver is starting to flag your IP/domain as spam — a signal
worth catching before it becomes a delivery cliff.

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

## Bounce / DSN setup

Async bounce ingestion + auto-suppression is **opt-in**. Without
`IRIS_BOUNCE_DOMAIN`, the renderer doesn't emit the inbound catcher or
the outbound VERP rewrite, the dsn-stream consumer logs `disabled` at
boot, and the `/observability/dsns` page renders empty.

> Synchronous bounces (5xx during the SMTP transaction) already work
> without any of this — they flow through the existing log-stream
> consumer as `Bounce` events on `/observability/logs`. This setup is
> purely for the *async* case: receiver accepted at SMTP, then later
> sent a [multipart/report][rfc3464] back as inbound mail.

### Prerequisites

Before you start you'll need:

- A **DNS zone** you control for the parent sending domain (you'll
  add records to a subdomain).
- **Inbound :25 connectivity** to the host running kumomta. If your
  cloud provider blocks inbound :25 (rare for outbound-blocked, but
  some firewalls do both), DSNs will simply never reach you. Confirm
  with `nc -vz <kumomta-host> 25` from off-network.
- A **VMTA + sender domain that's already DKIM-signed** by iris. The
  renderer's hook order guarantees DKIM runs against the *original*
  sender domain *before* VERP rewrites the envelope, but the DKIM key
  has to exist — generate or import on the **DKIM** page first.
- The **log-stream Redis** (`IRIS_LOGSTREAM_REDIS_URL`) and the
  **TimescaleDB** instance the rest of iris uses. The DSN pipeline
  reuses both — there's no separate Redis or new schema beyond the
  `dsn_event` hypertable that ent + the migration in
  `sql/0005_dsn_event_hypertable.sql` create on first boot.

### 1. Pick a mode and a bounce subdomain shape

The bounce pipeline has two modes. Pick the one that matches your
sending topology:

#### Single-domain mode — one organizational domain

You send from `news@example.com`, `alerts@app.example.com`,
`team@docs.example.com`. All of them share `example.com` as the
organizational domain, so one bounce subdomain at the org level
satisfies DMARC-relaxed alignment for everyone.

```bash
IRIS_BOUNCE_DOMAIN=bounces.example.com
# IRIS_BOUNCE_SENDER_DOMAINS unset
```

The renderer rewrites MAIL FROM to `b+TOKEN@bounces.example.com` on
every outbound, regardless of which subdomain the From: came from.
Cheapest setup: one MX record, one SPF record.

#### Multi-domain mode — cross-organizational sending

You send from `news@test-1.com` *and* `sender@test2.com`. These are
different organizational domains, so a single bounce subdomain
can't align with both — Gmail would tag one of them with "via". The
fix is one bounce subdomain *per* sender, derived by convention:

```bash
IRIS_BOUNCE_SENDER_DOMAINS=test-1.com,test2.com
IRIS_BOUNCE_DOMAIN_PREFIX=bounces   # default; override only if your DNS already uses a different label
# IRIS_BOUNCE_DOMAIN may be left set as a single-domain fallback,
# but multi-mode wins when both are configured.
```

The renderer derives:

| From: domain | Rewritten MAIL FROM |
|---|---|
| `news@test-1.com` | `b+TOKEN@bounces.test-1.com` |
| `sender@test2.com` | `b+TOKEN@bounces.test2.com` |
| anything else (sender not in the list) | left unrewritten — better than producing an unaligned MAIL FROM for an unmanaged domain |

DKIM still signs with the From-domain key (configured on the **DKIM**
page). Hook order in the rendered Lua guarantees DKIM runs *before*
VERP rewrites the envelope.

> Quick decision rule: if `dig +short SOA` for any two of your sending
> domains returns different SOAs, you're cross-org → use multi-domain
> mode.

**Don't** use a completely unrelated bounce domain (`bounces-randomname.io`)
in single-domain mode — every Gmail message will carry a "via
bounces-randomname.io" tag, because DMARC's relaxed-alignment rule
treats only same-organization domains as aligned.

### 2. DNS records

Each bounce subdomain (one in single-domain mode, N in multi-domain
mode) needs an MX and an SPF record. Examples in BIND zone syntax;
adapt verbatim to your DNS provider's UI.

#### Single-domain mode

```dns
;; Where receivers should send DSNs. Point at the host running kumomta.
;; If your kumomta runs behind NAT, use the publicly-resolvable name.
bounces.example.com.   IN MX   10 mx.your-kumomta-host.example.com.

;; SPF on the *bounce subdomain* — this is what makes DMARC-relaxed
;; alignment pass for outbound mail. List the IPs your kumomta sends
;; from. Use -all (hard fail) once you've verified everything works;
;; ~all (soft fail) is fine while you're testing.
bounces.example.com.   IN TXT  "v=spf1 ip4:203.0.113.10 ip4:203.0.113.11 -all"

;; Optional but recommended: explicit MX hostname A/AAAA so receivers
;; don't fall back to the bounce subdomain's address record.
mx.your-kumomta-host.example.com.   IN A    203.0.113.10
mx.your-kumomta-host.example.com.   IN AAAA 2001:db8::10
```

#### Multi-domain mode

Repeat the MX + SPF pair for **every** sender domain. They all point
at the same kumomta host — receivers route DSNs to the right MX
based on the bounce subdomain in the recipient address.

```dns
;; First sending domain
bounces.test-1.com.   IN MX   10 mx.your-kumomta-host.example.com.
bounces.test-1.com.   IN TXT  "v=spf1 ip4:203.0.113.10 ip4:203.0.113.11 -all"

;; Second sending domain — same MX target, separate SPF copy on its own zone
bounces.test2.com.    IN MX   10 mx.your-kumomta-host.example.com.
bounces.test2.com.    IN TXT  "v=spf1 ip4:203.0.113.10 ip4:203.0.113.11 -all"

;; Shared MX hostname (one A/AAAA pair, served from your operator zone)
mx.your-kumomta-host.example.com.   IN A    203.0.113.10
mx.your-kumomta-host.example.com.   IN AAAA 2001:db8::10
```

Each sender's parent zone (`test-1.com`, `test2.com`) needs the
records added — usually a different DNS provider per zone if the
domains belong to different teams. Iris doesn't manage DNS; this is
a one-time per-domain setup that mirrors what any ESP does for
their customers.

**Verification** (from anywhere). In multi-domain mode, run the
first three checks for **every** bounce subdomain you configured.

```bash
# 1. MX resolves
dig +short MX bounces.example.com         # single-domain
dig +short MX bounces.test-1.com          # multi-domain — repeat per sender
dig +short MX bounces.test2.com
#   each → 10 mx.your-kumomta-host.example.com.

# 2. SPF is in place and correct (runs against EACH bounce subdomain
#    because that's where receivers do the SPF lookup).
dig +short TXT bounces.test-1.com
dig +short TXT bounces.test2.com
#   each → "v=spf1 ip4:203.0.113.10 ip4:203.0.113.11 -all"

# 3. The MX target has reverse DNS that matches its forward record
#    (most receivers refuse SMTP from hosts without consistent rDNS).
dig +short -x 203.0.113.10
#   → mx.your-kumomta-host.example.com.

# 4. :25 is reachable (run from off-network, NOT from the host itself)
nc -vz mx.your-kumomta-host.example.com 25
```

**On parent-domain records:** you don't need to change anything on
`example.com` itself. DKIM continues to sign with the parent-domain
selector (operators set this on the **DKIM** page); the renderer's
outbound hook signs with that key *before* rewriting MAIL FROM, so
receivers see DKIM-passing mail with `d=example.com` and
SPF-passing mail authenticated against `bounces.example.com`. Both
align with the From domain under DMARC's relaxed mode.

**On DMARC:** if you publish a DMARC record on the parent
(`_dmarc.example.com`), make sure its `aspf=` is `r` (relaxed,
default) not `s` (strict). With strict alignment, DMARC requires the
SPF domain to *exactly* match the From domain, which a bounce
subdomain by definition won't satisfy.

### 3. Generate a VERP secret and configure iris

The VERP secret is what makes the inbound catcher able to verify that
a DSN's local-part really came from your outbound. Generate something
strong, store it in your secret manager, and inject it as an env var
on the admin-service binary.

```bash
# 32 random bytes (64 hex chars) is plenty.
IRIS_VERP_SECRET=$(openssl rand -hex 32)
echo "$IRIS_VERP_SECRET"   # save this to your secret manager NOW
```

> **Don't commit the secret.** It's not a "is this token real" key
> like JWT — it's the only thing standing between you and forged
> bounces that auto-suppress arbitrary recipients. Treat it like
> a database password.

#### Docker Compose

The compose file already declares the env vars with dev-friendly
defaults; for local testing you can leave them as-is. For staging /
prod, override via a `.env` file in the same directory as
`docker-compose.yaml`:

```bash
# deploy/.env  (gitignored)
IRIS_VERP_SECRET=<paste the openssl output here>

# --- Pick ONE mode below ---

# Single-domain mode (one organizational domain):
IRIS_BOUNCE_DOMAIN=bounces.example.com

# OR multi-domain mode (cross-org senders) — wins when both are set:
# IRIS_BOUNCE_SENDER_DOMAINS=test-1.com,test2.com
# IRIS_BOUNCE_DOMAIN_PREFIX=bounces        # optional, defaults to "bounces"

# Optional — auto-suppression knobs (defaults are conservative)
IRIS_BOUNCE_AUTO_SUPPRESS=true
IRIS_SOFT_BOUNCE_THRESHOLD=3
IRIS_SOFT_BOUNCE_WINDOW_HOURS=168
```

Then bring up the stack:

```bash
cd deploy
make rebuild s=admin-service
docker compose logs admin-service | grep -i dsn
#   → dsnstream: starting consumer stream=kumo.dsns group=iris-dsn ...
```

#### Host-native systemd

Add to your service unit's `EnvironmentFile=` target (typically
`/etc/iris/iris.env`):

```ini
IRIS_VERP_SECRET=<paste from secret manager>

# Single-domain mode:
IRIS_BOUNCE_DOMAIN=bounces.example.com
# OR multi-domain mode (cross-org senders):
# IRIS_BOUNCE_SENDER_DOMAINS=test-1.com,test2.com
# IRIS_BOUNCE_DOMAIN_PREFIX=bounces

IRIS_BOUNCE_AUTO_SUPPRESS=true
IRIS_SOFT_BOUNCE_THRESHOLD=3
IRIS_SOFT_BOUNCE_WINDOW_HOURS=168
```

Then `systemctl restart iris-admin` and confirm:

```bash
journalctl -u iris-admin -n 50 | grep -i dsn
```

#### Kubernetes / scratch

Wire `IRIS_VERP_SECRET` from a `Secret`, the rest from a `ConfigMap`.
A minimal manifest fragment:

```yaml
env:
  - name: IRIS_BOUNCE_DOMAIN
    value: bounces.example.com
  - name: IRIS_VERP_SECRET
    valueFrom:
      secretKeyRef: { name: iris-verp, key: secret }
  - name: IRIS_BOUNCE_AUTO_SUPPRESS
    value: "true"
```

### 4. Apply the rendered policy

The new env vars only take effect when iris regenerates `init.lua` and
kumomta reloads it. Two ways:

**Via the UI** (recommended for first-timers — you can see what
changed before pushing it live):

1. Open `/policy/editor`.
2. Click **Regenerate** — the Preview pane now shows the new
   `init.lua`. Confirm these markers are present:
   - `-- ===== bounce / DSN pipeline (mode: single-domain (legacy)) =====`
     **or** `(mode: multi-domain) =====` — should match what you
     configured.
   - `local BOUNCE_DOMAINS = { ["bounces.example.com"] = true, … }`
     — the catcher's accept-list. In multi mode you should see one
     entry per sender domain.
   - In multi mode only, `local BOUNCE_SENDER_TO_BOUNCE = { ["test-1.com"]
     = "bounces.test-1.com", … }` — the per-sender lookup the outbound
     hook uses.
   - `kumo.on('make.dsn_xadd', function(...)` — the inbound catcher.
   - `kumo.on('get_listener_domain', ...)` with `relay_to = true` for
     every bounce subdomain.
   - Inside the `smtp_client_message_sending` hook,
     `msg:set_sender(string.format('b+%s.%s@%s', prefix, mid, bounce_dom))`
     — the VERP rewrite, with `bounce_dom` looked up via
     `BOUNCE_SENDER_TO_BOUNCE[from_dom]`.
3. Click **Apply**. The SHA-256 of the active policy should change.

**Via the API** (for automation):

```bash
# Render and inspect (no side effects)
curl -sS http://localhost:8000/v1/policy/render | jq -r .lua | \
  grep -E "BOUNCE_DOMAIN|make\.dsn_xadd|set_sender"

# Apply
curl -sS -X POST http://localhost:8000/v1/policy/apply \
  -H 'Content-Type: application/json' \
  -d '{"note":"enable bounce pipeline"}'
#   → {"sha256":"...","applied_at":"..."}
```

KumoMTA hot-reloads on its 10-second epoch poll — give it ~10 seconds
to pick up the change, or restart the kumomta process for instant
effect.

### 5. End-to-end verification

#### Outbound — confirm MAIL FROM is being VERP'd

Send any test message through your kumomta and watch the SMTP
transaction. The clearest signal is the kumomta log:

```bash
# Send a test
swaks --to test@your-other-mailbox.example.com \
      --from sender@example.com \
      --server localhost:2525 \
      --header "Subject: verp test"

# Watch the kumomta dispatch log for the rewritten envelope
docker logs iris-kumomta 2>&1 | grep -E "MAIL FROM" | tail -5
#   → MAIL FROM:<b+a3f8b9d2c1e7f4a6.cd7b9a40e3@bounces.example.com>
```

Confirm the receiver's `Return-Path:` header in the delivered message
matches what you sent. View source on the message:

```
Return-Path: <b+a3f8b9d2c1e7f4a6.cd7b9a40e3@bounces.example.com>
```

If you see your unrewritten `From:` address as the Return-Path, the
hook didn't fire — re-check that you applied the policy and that
`IRIS_BOUNCE_DOMAIN` was set when the admin-service started.

#### Inbound — confirm DSNs land in `dsn_event`

The cleanest test is to send a real bounce: address a message to
something that doesn't exist on a receiver you control (e.g.
`nobody-real@your-other-domain.example`), wait for the DSN to come
back. Or synthesise one — a complete RFC 3464 sample is in
`backend/pkg/dsnstream/parse_test.go` (`sampleDSNGmail`).

```bash
# Using the sample as a starting point — generate a VERP token for a
# fake message_id, then deliver as a DSN to your kumomta. The format
# is "<hex16>.<message_id>" where hex16 is the first 8 bytes of
# HMAC-SHA256(secret, message_id), lowercase hex.
MSGID="abc12345deadbeef"
HMAC16=$(printf '%s' "$MSGID" \
  | openssl dgst -sha256 -mac HMAC -macopt key:"$IRIS_VERP_SECRET" -hex \
  | awk '{print $NF}' \
  | cut -c1-16)
ENV_RCPT="b+${HMAC16}.${MSGID}@bounces.example.com"
echo "Envelope recipient: $ENV_RCPT"

# Deliver via swaks. Use --data with a multipart/report body.
swaks --to "$ENV_RCPT" --from "<>" \
      --server localhost:2525 \
      --data path/to/sample-dsn.eml

# Confirm the row landed
psql -d iris -c "
  SELECT received_at, action, status, category, mail_class, final_recipient
  FROM dsn_event
  ORDER BY received_at DESC
  LIMIT 5;"
#   → action=failed, status=5.1.1, category=unknown_user, mail_class=marketing
```

#### Auto-suppression — confirm hard bounces become suppressions

```bash
psql -d iris -c "
  SELECT address, reason, note, created_at
  FROM suppression_entries
  WHERE reason IN ('hard_bounce','soft_bounce','expired')
  ORDER BY created_at DESC
  LIMIT 10;"
#   → address=alice@external.test, reason=hard_bounce,
#     note='status=5.1.1 | category=unknown_user | diag=smtp; 550 ...'
```

Then load `/observability/dsns` in the UI. The row you just inserted
should appear with the right colour-coded category, the message_id
should be a clickable link to the Logs page, and **Details** should
open a modal showing the parsed structure plus the embedded headers.

### 6. Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `dsnstream: disabled` at boot | Both `IRIS_BOUNCE_DOMAIN` *and* `IRIS_BOUNCE_SENDER_DOMAINS` are empty, or `IRIS_LOGSTREAM_REDIS_URL` is unset | Set one of the bounce-config vars plus the Redis URL. The DSN pipeline shares Redis with the log-stream consumer. |
| Render fails with a VERP-secret error | `IRIS_VERP_SECRET` is empty or under 16 bytes | Generate a new one with `openssl rand -hex 32`. |
| KumoMTA rejects the rendered policy with `unknown field 'require_tls'` | You're running an older renderer — this was removed when the bounce-domain listener-domain rule was added | Pull latest, redeploy admin-service. |
| Outbound mail still has the original From: as Return-Path | Policy hasn't been re-applied since you set the bounce vars, OR the From: domain isn't in `IRIS_BOUNCE_SENDER_DOMAINS` (multi mode) | UI → Policy Editor → Apply, or POST `/v1/policy/apply`. Wait 10s for kumomta to hot-reload. In multi mode, only senders explicitly listed get rewritten; everything else passes through unchanged. |
| One sender's outbound rewrites correctly, another doesn't | (Multi mode) The "broken" sender domain isn't in `IRIS_BOUNCE_SENDER_DOMAINS`, OR the lookup failed because of casing in operator config (env reader lowercases; if you set the value programmatically, do too) | Add the missing domain to the list, re-apply policy. Inspect the rendered Lua's `BOUNCE_SENDER_TO_BOUNCE` table — it should contain a lowercase key for every From: domain. |
| DSNs reach the bounce domain but `dsn_event` stays empty | `XADD` to `kumo.dsns` is failing — usually a Redis connectivity issue | `docker exec iris-kumomta redis-cli -h redis ping` (or whatever your topology is). Check kumomta logs for `dsn: xadd failed`. |
| KumoMTA rejects DSNs at the bounce subdomain with 5xx | (Multi mode) That subdomain isn't in the rendered listener-domain map | Confirm the sender domain appears in `IRIS_BOUNCE_SENDER_DOMAINS`; re-apply policy; inspect `BOUNCE_DOMAINS` in the active Lua. |
| Rows appear in `dsn_event` but `message_id_ref` is empty | VERP token didn't validate. Either the secret rotated since the message was sent, or a receiver mangled MAIL FROM | The fallback (embedded `Message-ID` from the original) usually still populates it. If both are empty, the DSN was probably backscatter — verify by inspecting `extra_json.envelope_recipient`. |
| Hard bounces aren't auto-suppressing | `IRIS_BOUNCE_AUTO_SUPPRESS=false`, or an existing higher-severity row already exists for that address | Check the existing suppression's reason: operator-set (`manual`, `complaint`) wins over auto. |
| Gmail shows "via bounces.something" on every message | Bounce domain isn't a subdomain of the From: domain (single-domain mode used for cross-org senders), OR DMARC has `aspf=s` (strict) | Switch to multi-domain mode (`IRIS_BOUNCE_SENDER_DOMAINS`); switch DMARC to `aspf=r` (relaxed, the default). |
| Receivers reject outbound mail at MAIL FROM with SPF failure | SPF on one of the bounce subdomains isn't published or doesn't include your sending IP | (Multi mode) Run the SPF `dig` for *each* sender domain — easy to miss one. Verify the sending IP is listed in every record. |

If something else breaks: the consumer logs verbosely under
`dsnstream: …` and parse failures land in the `kumo.dsns.dlq` Redis
stream (`XRANGE kumo.dsns.dlq - +` to inspect).

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

Outbound deliverability hygiene (set in **Global Settings**, not env): an **Outbound EHLO hostname** field sets a default egress `ehlo_domain` applied at the egress-source level — including the implicit `default` source used when no routing rule matches — so KumoMTA never announces the bare system hostname (`rspamd HFILTER_HELO_5`, or `450 Helo command rejected: Host not found`); a per-VMTA HELO name still overrides it. The renderer also auto-adds `Date:` and `Message-ID:` to submitted/injected mail that lacks them (`rspamd MISSING_DATE` / `MISSING_MID`), before DKIM signing. A **Delivery retries** card tunes the outbound `retry_interval` / `max_retry_interval` / `max_age` for messages in TransientFailure (blank = KumoMTA defaults: retry every 20m, doubling, give up after 7d). All take effect on the next **Apply**.
| `IRIS_BOUNCE_DOMAIN` | *(unset → legacy single-domain mode disabled)* | **Single-domain mode**: one bounce hostname (e.g. `bounces.example.com`) every outbound funnels through. Use only when all your sending domains share an organizational domain so DMARC's relaxed alignment treats one bounce subdomain as aligned with all of them. Ignored when `IRIS_BOUNCE_SENDER_DOMAINS` is set. |
| `IRIS_BOUNCE_SENDER_DOMAINS` | *(unset)* | **Multi-domain mode**: comma-separated list of From: domains this kumomta hosts (`test-1.com,test2.com,…`). The renderer derives one bounce subdomain per entry by the convention `<prefix>.<sender>` and rewrites MAIL FROM per-message to the aligned subdomain. Required when sending from cross-org domains. Operator must publish DNS MX + SPF for *each* derived bounce subdomain — see *Bounce / DSN setup* below. Multi-mode wins when both this and `IRIS_BOUNCE_DOMAIN` are set. |
| `IRIS_BOUNCE_DOMAIN_PREFIX` | `bounces` | Leading label prepended to each `IRIS_BOUNCE_SENDER_DOMAINS` entry. Override only if your DNS scheme already uses a different prefix (e.g. `rcpt`, `mailer`). Lower-cased on use. |
| `IRIS_VERP_SECRET` | *(required when bounce domain is set)* | HMAC-SHA256 key used to sign and verify VERP tokens. Must be ≥16 bytes; the renderer refuses to emit the rewrite hook with a shorter key. **Never log this value.** |
| `IRIS_DSNSTREAM_NAME` | `kumo.dsns` | Redis Streams key the DSN catcher XADDs to and the iris consumer reads from |
| `IRIS_DSNSTREAM_GROUP` | `iris-dsn` | consumer group |
| `IRIS_DSNSTREAM_WORKERS` | `2` | parallel DSN consumers (lower than logstream — bounces are far less bursty) |
| `IRIS_BOUNCE_AUTO_SUPPRESS` | `true` | set `false` to stop adding bounces to the suppression list automatically; the dsn_event table still populates so the Bounces UI is unaffected |
| `IRIS_SOFT_BOUNCE_THRESHOLD` | `3` | how many soft (4.x.x) bounces a recipient can accumulate within the window before iris suppresses them |
| `IRIS_SOFT_BOUNCE_WINDOW_HOURS` | `168` (7d) | sliding window for the soft-bounce threshold |
| `IRIS_BOUNCE_TOKEN_TTL` | `720h` (30d) | reserved — DSNs older than this should be treated as backscatter (not yet enforced; see roadmap) |
| `IRIS_GEOIP_DB_PATH` | `/var/lib/iris/dbip-country-lite.mmdb` | Country `.mmdb` for the **login firewall**'s REGION rules. Any MaxMind-DB-format file with a `country.iso_code` field works — the free [DB-IP IP-to-Country Lite](https://db-ip.com/db/download/ip-to-country-lite) db or a MaxMind GeoLite2-Country db. **Auto-downloaded** on startup by default (see below). Default lives in `/var/lib/iris` (the systemd unit's `StateDirectory=`, writable under `ProtectSystem=strict`); don't point it at `/opt/kumomta/etc`, which the hardened unit mounts read-only. Optional overall: with auto-update off and no file, region rules fail open (logged); IP and time-window rules need no database. |
| `IRIS_GEOIP_AUTO_UPDATE` | `true` | Auto-download the current month's DB-IP database to `IRIS_GEOIP_DB_PATH` on boot, hot-swapping it into the firewall without a restart, and re-check on an interval. Set `false` for air-gapped hosts that ship the file out-of-band. Failures are best-effort — they never block boot or login. |
| `IRIS_GEOIP_UPDATE_INTERVAL` | `24h` | How often the updater re-checks for a new monthly release. |
| `IRIS_GEOIP_DOWNLOAD_URL` | *(DB-IP free)* | Override the download URL template (`%s` = `YYYY-MM`) for a mirror or a paid DB-IP/MaxMind edition. Body must be a gzipped `.mmdb`. |
| `IRIS_METRICS_LISTEN` | `127.0.0.1:9090` | bind for the Prometheus `/metrics` listener; set to `off` to disable, or to `0.0.0.0:9090` when behind a reverse proxy / Docker port-forward |
| `IRIS_PROMETHEUS_URL` | *(unset → /v1/dashboard/* return 503)* | base URL of a Prometheus instance the admin-service queries on the operator UI's behalf for the `/analytics` dashboard. In compose: `http://prometheus:9090`. |
| `IRIS_AUTH_ACCESS_SECRET` / `_REFRESH_SECRET` | *(refused if placeholder)* | JWT HS512 secrets — the binary refuses to start with the shipped placeholder values |
| `IRIS_MFA_SECRET_KEY` | *(unset → TOTP enrollment disabled)* | AES-GCM key (base64 of 16/24/32 bytes) encrypting TOTP secrets at rest. `openssl rand -base64 32`. Rotating it invalidates existing TOTP enrollments. WebAuthn + backup codes don't need it. |
| `IRIS_WEBAUTHN_RP_ID` | *(unset → passkeys disabled)* | WebAuthn Relying Party ID — the registrable domain (e.g. `kmx.jobs.bg`). |
| `IRIS_WEBAUTHN_RP_ORIGINS` | *(unset)* | Comma-separated full origins users hit (e.g. `https://kmx.jobs.bg:8443`). Must match the browser's address bar exactly. |
| `IRIS_WEBAUTHN_RP_DISPLAY_NAME` | `Iris` | Name shown by the authenticator during passkey enrollment. |

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

### Async bounce pipeline (DSN handling)

Synchronous bounces — receiver returns a 5xx during the SMTP transaction —
already flow through the existing log-stream consumer as `Bounce` events.
The DSN pipeline handles the asynchronous case: receiver accepted the
message at the SMTP layer, then *later* could not deliver and sent a
[multipart/report][rfc3464] back as inbound mail.

Three things have to be true for the loop to close cleanly:

1. **The bounce reaches us.** `IRIS_BOUNCE_DOMAIN` (single-domain) or
   `IRIS_BOUNCE_SENDER_DOMAINS` (multi-domain) configures one or more
   bounce subdomains. The renderer emits a `get_listener_domain` rule
   that accepts mail for each one, and a `make.dsn_xadd` custom_lua
   queue constructor that XADDs the raw RFC 822 onto a Redis stream.
   Multi-domain mode is what makes cross-org sending work — each
   sender's bounces flow back to a DMARC-aligned subdomain
   (`bounces.<sender>`), all funnel into the same Redis stream and
   `dsn_event` table downstream.
2. **We can match the bounce to the original send.** The renderer also
   emits an outbound `smtp_client_message_sending` hook that rewrites
   `MAIL FROM` to a VERP token: `b+<hex16>.<message_id>@<bounce_domain>`,
   where `hex16` is the first 8 bytes of `HMAC-SHA256(IRIS_VERP_SECRET,
   message_id)`. When the DSN comes back, the catcher writes the
   envelope-recipient to Redis and the consumer parses out the token,
   validates the HMAC, and recovers the `message_id` for correlation.
   The Lua emitter and the Go verifier (`pkg/verp`) share a pinned test
   vector so a future drift is caught at build time.
3. **We do something useful with it.** The consumer (`pkg/dsnstream`)
   parses the multipart/report per RFC 3464, classifies the
   enhanced-status code into one of 14 stable buckets via
   `pkg/bounceclass` (e.g. `unknown_user`, `mailbox_full`,
   `policy_block`, `reputation_block`, `auth_failed`, …), denormalises
   `mail_class` / `tenant` / `campaign` from the originating LogEvent,
   and persists into the `dsn_event` TimescaleDB hypertable.
   Auto-suppression then runs: hard bounce (5.x.x) → suppress with
   reason `hard_bounce`; soft bounce (4.x.x) → suppress only after
   `IRIS_SOFT_BOUNCE_THRESHOLD` accumulate within
   `IRIS_SOFT_BOUNCE_WINDOW_HOURS`; `Action: expired` (kumomta gave up
   after retry exhaustion) → suppress as `expired`. Operator-set
   suppressions (`manual`, `complaint`) are protected by a severity
   ladder so automation never silently overrides them.

If a DSN arrives without a valid VERP token (older sends, shared
bounce mailbox, intermediary stripped MAIL FROM), the parser falls
back to the embedded `Message-ID` from the `message/rfc822`
sub-part — and from there to the `X-Kumo-Mail-Class` header on the
embedded original — so even partially-formed bounces still get
classified.

DKIM signing happens in the same `smtp_client_message_sending` hook,
ordered *before* the VERP rewrite so the signature is computed against
the original sender domain. SPF on the bounce subdomain plus DMARC's
relaxed alignment is what keeps Gmail from showing the
"via bounces.example.com" indicator on every message.

[rfc3464]: https://datatracker.ietf.org/doc/html/rfc3464

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
- **MFA (optional, self-service).** TOTP, WebAuthn passkeys, and single-use
  backup codes. Login is two-step: after the password, an enrolled account
  gets a short-lived challenge token (5 min) instead of session tokens, and
  must verify a factor to obtain them. TOTP secrets are AES-GCM encrypted at
  rest (`IRIS_MFA_SECRET_KEY`); backup codes are bcrypt-hashed and consumed
  on use; WebAuthn tracks the signature counter for clone detection. Admins
  can reset a user's MFA (lost device); WebAuthn needs `IRIS_WEBAUTHN_RP_*`.
- **Login firewall.** Per-user and global rules gate authentication by
  IP/CIDR, country, or time window (blacklist / whitelist), evaluated
  before the password compare so blocked sources never reach the
  credential path. Fails open on indeterminate attributes (no client IP,
  GeoIP DB absent) to avoid lockouts, and Create/Update refuse a rule
  that would lock out the acting operator unless explicitly acknowledged.
- **Audit.** Every admin operation produces an `audit_entry` row
  (actor / IP / status / duration); the writer is async + batched,
  with clean shutdown draining its queue.
- **Lua injection.** Every operator-controlled value emitted into
  Lua flows through `MustLuaString`. Suppression values don't reach
  Lua at all (Redis-backed lookup), so that surface is structurally
  closed.
- **Secrets.** JWT secrets refused if placeholder. DKIM private keys
  stored mode 0640 (group-readable for the kumomta uid only).
- **Write-only credentials.** DNS-01 provider credentials
  (`/security/dns-providers`) are write-only: the API accepts them on
  save but never returns the values — list/save responses carry only
  `configured_keys` (which fields are set), so a secret can't be read
  back out once entered. Edits merge over the stored config (blank field
  = keep existing), so changing one credential never exposes or requires
  re-typing the others.

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
- Bounce-pipeline TTL enforcement (`IRIS_BOUNCE_TOKEN_TTL`) — drop
  DSNs whose VERP token is older than the configured horizon.
- Per-mail-class auto-suppression overrides — let operators pick
  tighter thresholds for transactional vs marketing classes from
  the UI rather than env.
- Bounce dashboard tile on `/analytics`: bounce rate stratified
  by mail class (and, in multi-domain mode, by sender domain) with
  a sparkline.
- Move `IRIS_BOUNCE_SENDER_DOMAINS` from env to a UI-managed table
  so adding a new sending domain is a click rather than a redeploy.

---

## License

Not yet selected. PRs welcome; a sense of humour about message
delivery is encouraged but not required.

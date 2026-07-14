# KumoMTA Cluster Support — Architecture & Implementation Plan

Status: P1 IMPLEMENTED (2026-07-14) — node registry, iris-agent (mTLS
stage/activate/health + kumod proxy), cluster CA CLI (`iris cluster init-ca` /
`issue-cert`), transport abstraction with rolling multi-node ApplyConfig, and
the Cluster Nodes UI. Online CSR enrollment (token schema already in place),
vmtas.node_id + proxy-aware rendering (P2), and observability node labels (P3)
are next.

## 1. Goal

Run iris against a cluster of N KumoMTA nodes instead of one co-located instance:

- Any node can accept submission (ESMTP or HTTP inject).
- Routing stays exactly as it is today: routing rules match mail class (header /
  recipient / sender-IP) and select a VMTA or VMTA group.
- A VMTA (egress IP) now *lives on a specific node*. A message received on
  node1 whose route resolves to a VMTA on node2 must leave the internet from
  node2's IP.
- Logging, message correlation and observability must not degrade.
- Priorities: **security first, observability second**.

## 2. The core decision: proxy egress, not message forwarding

The question from the design discussion: *"do we route messages between nodes
with kumo-proxy? That makes no sense — we have routing logic based on mail
class."*

Clarification that resolves it: **kumo-proxy does not route anything.** The
routing decision is unchanged and stays where it is today — in the
iris-generated Lua policy (`classify_mail` → `select_pool` → `tenant` meta →
`egress_pool`). What changes is only the *last TCP hop*: when the selected
VMTA's IP is physically bound on another node, the outbound SMTP connection is
tunneled through kumo-proxy (SOCKS5) on that node, so packets leave from the
correct IP. The message itself is **never re-queued on another node**.

So for the example scenario:

1. Mail arrives at the submission listener on **node1**.
2. Node1's policy (identical on every node) classifies it → mail class `bulk`
   → routing rule → egress pool `vmta-7` (which lives on **node2**).
3. The message is queued **on node1** (node1's spool, node1's queues).
4. At delivery time node1 opens the connection *through kumo-proxy on node2*,
   with `source_address = vmta-7's IP`. The receiving MX sees node2's IP,
   PTR, EHLO — exactly as if node2 had sent it.
5. Every lifecycle log record (Reception → TransientFailure → Delivery/Bounce)
   is emitted by **node1** into the shared Redis stream with the same message
   `id`. Correlation is intact by construction.

### Why not the alternatives

| Option | Verdict | Reason |
|---|---|---|
| **A. kumo-proxy egress (chosen)** | ✅ | Official KumoMTA cluster model. Whole message lifecycle stays on one node → single coherent log trail, no dedup, VERP/DKIM/suppression unchanged. Identical config on all nodes. |
| B. SMTP relay: node1 re-queues to node2 | ❌ | Second Reception on node2 with a **new message id** → broken correlation, duplicated Reception records, double spool hop, VERP rewrite runs twice, suppression/rspamd re-run or must be special-cased. Exactly the observability breakage we must avoid. |
| C. Load-balance submissions to the "right" node up front | ❌ | Impossible in general: the routing decision depends on message content (mail class header), which is only known after reception. |

Cost of the chosen model: cross-node delivery traffic transits the private
network twice (node1→node2→internet), and a VMTA's deliveries depend on its
owning node being up. Both are acceptable and are the documented KumoMTA
trade-offs.

## 3. Target topology

```
                        ┌──────────────────────────────┐
   senders ──────────▶  │  LB / DNS (ESMTP 25/587,     │
                        │  HTTP inject)                │
                        └───────┬──────────┬───────────┘
                                ▼          ▼
                     ┌────────────┐   ┌────────────┐
                     │  node1     │   │  node2     │      ... nodeN
                     │  kumod     │   │  kumod     │
                     │  kumo-proxy│◀─▶│  kumo-proxy│   (SOCKS5, private net only)
                     │  iris-agent│   │  iris-agent│   (mTLS control plane)
                     │  local spool   │  local spool
                     └─────┬──────┘   └─────┬──────┘
                           │  XADD / EXISTS │
                           ▼                ▼
                   ┌─────────────────────────────┐     ┌───────────┐
                   │ Redis (streams, suppression,│     │ tsa-daemon│◀─ all nodes
                   │ cluster throttles) — HA     │     │ (shared)  │   publish/subscribe
                   └──────────────┬──────────────┘     └─────▲─────┘
                                  │ consumer group iris-logstream │
                                  ▼                              │
                   ┌─────────────────────────────┐               │
                   │ iris (API, UI, workers)     │───────────────┘
                   │ + Prometheus                │  renders & distributes config,
                   └─────────────────────────────┘  scrapes metrics per node
```

Components:

- **kumod, N nodes** — *identical* iris-generated policy on every node (the
  KumoMTA-recommended model; also what makes shaping and routing coherent).
  Per-node local RocksDB spool (unchanged). Node identity injected as a tiny
  per-node prelude (`NODE_NAME`), not by diverging the policy.
- **kumo-proxy on every node that owns egress IPs** — listens **only on the
  private cluster network**. kumo-proxy has *no authentication*; isolation is
  mandatory (see §6).
- **Redis (existing)** — gains three cluster roles: shared mail-event streams
  (already multi-producer safe), suppression lookups (unchanged), and
  **`kumo.configure_redis_throttles`** so per-egress-path rate limits and
  connection caps are enforced cluster-wide, not per node. Redis becomes
  critical shared infrastructure → needs HA (Sentinel or managed) and ACLs.
- **tsa-daemon, shared** — one instance (or a small sub-cluster later). All
  nodes publish shaping events to it and subscribe to its overrides, so a
  block observed by node1 throttles node2 too. iris keeps rendering
  `iris-automation.toml` and now ships it to the TSA host.
- **iris-agent on every node** — new, small control-plane daemon (an `iris
  agent` subcommand of the existing binary). mTLS server. Responsibilities:
  receive/stage/activate config, reload/restart kumod, proxy admin API calls
  to the localhost-bound kumod HTTP listener, report health + applied
  checksum. This replaces today's "iris writes a local file" assumption.

## 4. What changes in iris

### 4.1 Data model

- New table `mta_nodes`: `id, name, agent_url, proxy_host, proxy_port,
  status (active|disabled|draining), fingerprint/cert serial, version,
  applied_checksum, last_seen_at`. New biz entity + repo + CRUD service
  (admin permission-gated), audit-logged.
- `vmtas.node_id` (nullable FK → `mta_nodes`). NULL = "local" for
  single-node/backward compat. VMTA IP uniqueness stays cluster-global.
- `mail_records.node` (TEXT, denormalized node name) + index — see §5.
- Config snapshot (`ConfigSnapshot`) gains the node list + proxy endpoints.

### 4.2 Config rendering (`kumo_config.go`)

- `writeEgressSources`: every egress source whose VMTA has a `node_id` gets
  `socks5_proxy_server = "<node.proxy_host>:<port>"` and
  `socks5_proxy_source_address = <vmta ip>`. **All nodes render sources
  identically** — even the owning node dials through its own local proxy.
  That keeps the policy byte-identical cluster-wide (one checksum, trivial
  drift detection) at the cost of a localhost proxy hop; KumoMTA recommends
  exactly this.
- New `writeClusterThrottles`: emit `kumo.configure_redis_throttles({node =
  REDIS_URL})` in init when clustering is enabled, so `max_message_rate` /
  `max_connection_rate` / connection caps become cluster-shared leases.
- `writeLogHook` / `configure_log_hook`: add `node` to the logged `meta` list;
  reception hooks add `msg:set_meta('node', NODE_NAME)`.
- Node identity: iris writes a 3-line per-node prelude file
  (`iris_node.lua`: `NODE_NAME`, optionally node-local listener binds) which
  the shared policy `dofile`s. The big policy stays identical everywhere.
- TSA: shaping publisher/subscriber blocks point at the shared TSA URL
  (already configurable as `TSAUrl`).

### 4.3 Config distribution & apply (replaces local `ApplyConfig`)

Today `FileKumoMTA.ApplyConfig` writes `/opt/kumomta/etc/policy/...` locally
and reloads. New flow, per node, via iris-agent over mTLS:

1. **Stage**: iris POSTs the bundle (policy Lua, `iris-base.toml`,
   `iris-warmup.toml`, `iris-automation.toml`, node prelude) with SHA-256
   checksums; agent writes to a staging dir, verifies checksums, runs the
   same Lua lint locally.
2. **Activate**: atomic rename into place (policy 0640 as today), then
   reload (`bump-config-epoch`) or restart (init-block changed — same
   `InitChecksum` logic as now).
3. **Verify**: agent polls kumod liveness + config epoch, reports back.
4. **Rolling apply**: iris applies one node at a time, health-gated;
   failure → stop the rollout, keep remaining nodes on the old config,
   surface per-node status. Restart-requiring changes drain-aware (optional
   suspend of new connections first).
5. Existing gates preserved: `PermServiceControl`, confirmation ID,
   `ServiceControlStore` serialization, audit events — now recorded per node.

Single-node installs keep working: a node with no `agent_url` uses today's
local file+reload path (the current adapter becomes the "local transport").

### 4.4 Runtime control fan-out

Everything in `kumomta_queue.go` / `kumomta_adapter.go` that hits one
`base_url` becomes per-node, proxied through the agent (kumod's HTTP listener
binds to localhost only):

- **Queue summary**: fan out `/metrics` scrape to all active nodes, aggregate
  `scheduled_by_domain` per domain, keep per-node breakdown available.
- **Suspend / resume / bounce queue**: fan out to all nodes (queues exist
  independently on each node); report per-node results, treat partial failure
  as a surfaced warning, not silent success.
- **Service control (reload/restart)**: per-node, plus "all nodes (rolling)".
- **HTTP injection** (`InjectV1` from iris's GreenArrow-compat listener):
  round-robin across healthy active nodes with failover — any node can accept,
  routing is content-based anyway.

### 4.5 UI

- New **Cluster** page (Infrastructure section): node list with status,
  version, applied config checksum vs expected (drift badge), last_seen,
  per-node queue depth/spool, actions (reload, restart, drain, disable),
  node enrollment flow.
- VMTA form: node selector (required when >1 active node).
- Config apply screen: per-node rollout progress.
- Mail Logs: `node` filter + column; message detail shows receiving node and
  (via VMTA→node mapping) egress node.

## 5. Observability (must not break)

What survives automatically because of the proxy model:

- **Correlation**: all records for a message share KumoMTA's message `id` and
  are emitted by one node. `mail_records.message_id` timeline grouping,
  `created`-based queue latency, VERP bounce correlation
  (`RecipientForMessageID`), X-KumoRef FBL tracing — all unchanged.
- **Event pipeline**: N nodes XADD to the same `iris.mail.events` stream; the
  `iris-logstream` consumer group already handles multiple producers. Same for
  DSN/DMARC/FBL/rspamd streams and the suppression keyspace.

What we add:

- `node` meta on every record (§4.2) → persisted `mail_records.node`,
  exposed in Logs UI, event JSON (`event_format.go`), and event processors.
- **Prometheus**: external Prometheus scrapes every kumod with a `node`
  label (deploy-side scrape config; document the cert/IP-SAN pitfall already
  hit once). iris metrics gain node where meaningful; widget catalog gets
  node-grouped variants of the `*_by_provider_and_source` and
  `egress_source_health_*` widgets; new curated widgets: per-node queue
  depth, per-node spool usage, proxy connection failures.
- **Attribution rule**: *receiving/queueing node* = `mail_records.node`;
  *egress node* = derived from `egress_source` → VMTA → `node_id`. Both
  visible per message.
- **Drift & health**: agent heartbeats (liveness, kumod status, applied
  checksum, spool disk); iris raises a visible warning on config drift or a
  stale node. Cluster health surfaces on the Overview dashboard.
- **Failure visibility**: if a VMTA's owning node is down, deliveries through
  it tempfail from other nodes — TSA + shaping back off automatically, and the
  cluster page must show "VMTA unreachable (node2 down)" rather than leaving
  operators to infer it from deferral graphs.

## 6. Security (top priority)

Current state to fix: kumod admin/inject API is plain HTTP, no auth,
"trusted because co-located". That assumption dies with clustering.

1. **Control plane — mTLS everywhere.** iris runs a minimal internal CA
   (or reuses an existing one): node enrollment = one-time bootstrap token →
   agent CSR → iris-signed client+server certs, short-lived, auto-rotated.
   iris↔agent is mutual TLS; agent authorizes only the iris CA.
2. **kumod listeners locked down.** kumod HTTP listener binds `127.0.0.1`
   only; all remote admin/inject/metrics access goes through the
   authenticated agent. ESMTP listeners keep existing relay allowlists.
3. **kumo-proxy isolation.** kumo-proxy is unauthenticated by design → it
   must listen only on a private, node-only network (dedicated VLAN or
   WireGuard mesh between nodes) with firewall rules restricting ingress to
   cluster peers. This is a hard deployment requirement, checked and shown
   on the cluster page (agent verifies the proxy bind address is private).
4. **Redis hardening.** Redis is now the cluster bus: require password +
   TLS, and ACL-scoped users — kumod's user limited to `XADD` on the
   `iris.*` streams, `EXISTS`/`GET` on `supp:*`, and the throttle keyspace;
   iris's user broader. Never exposed outside the private network.
5. **Secrets distribution.** The policy file embeds DKIM private keys and
   is now shipped to N hosts: transfer only over mTLS, at-rest 0640 (as
   today) with a dedicated user, agent never logs bundle contents, and the
   audit trail records who applied what where. (Follow-up, out of v1 scope:
   move DKIM keys out of the policy into agent-managed key files.)
6. **AuthZ & audit in iris.** New permissions: `cluster:read`,
   `cluster:write`; node CRUD/enroll/drain and every per-node apply or
   service action audit-logged with node identity. Existing
   confirmation-ID flow retained for destructive actions.
7. **Input validation.** Node registration validates agent URL scheme
   (https only), proxy host is an IP on a private range, names/slugs
   constrained; agent validates bundle checksums and rejects unsigned or
   stale (replay-protected: monotonic config generation number) applies.

## 7. Failure modes (documented behavior)

- **Node owning VMTA-X goes down**: messages routed to VMTA-X on other nodes
  tempfail at connect → normal retry schedule + TSA backoff; messages already
  spooled *on* the dead node wait for it to return (local spool model).
  VMTA failover / IP takeover is explicitly **out of scope for v1** (noted as
  future work: draining + reassigning `node_id` with IP re-plumbing).
- **Redis down**: suppression already fails open; cluster throttles degrade to
  node-local limits; log events buffer in kumod's log hook queue. Alert.
- **iris down**: data plane unaffected — nodes keep receiving and delivering
  on the last applied config (a deliberate property to preserve).
- **TSA down**: nodes keep static + warmup shaping; automation overrides
  pause. Alert.

## 8. Implementation phases

Each phase ships independently and keeps single-node deployments green.

1. **P1 — Node registry & agent (control plane).** `mta_nodes` model/CRUD/UI
   list, `iris agent` subcommand (mTLS, stage/activate/health, admin proxy),
   CA + enrollment. Migrate the single-node install to "cluster of one" via
   local transport. Tests: agent apply protocol, enrollment, authz.
2. **P2 — Multi-node rendering & egress.** `vmtas.node_id`, proxy-aware
   `writeEgressSources`, node prelude, `configure_redis_throttles`, shared
   TSA wiring, rolling apply. Tests: rendered-Lua golden tests for proxy
   sources and throttles; two-node docker-compose e2e (`-tags e2e`) proving
   the node1-submit/node2-egress scenario end-to-end including log records.
3. **P3 — Observability.** `node` meta → migration + Logs UI filter,
   per-node metrics fan-in (queue summary aggregation), node-grouped
   widgets, cluster health/drift on dashboards.
4. **P4 — Cluster operations.** Full cluster page (drain, per-node service
   control), queue-action fan-out, injection failover, partial-failure UX.
5. **P5 — Hardening.** Redis ACL/TLS rollout, private-bind verification for
   kumo-proxy, replay protection, cert rotation, security review pass
   (`security-reviewer`), load/failover testing, ops runbook.

## 9. Deployment & hardening guide (P5)

### Enrollment (online, recommended)

1. On the iris host: `iris cluster init-ca -dir /etc/iris/cluster-ca`, set
   `cluster.ca_dir: /etc/iris/cluster-ca`, and issue iris's own client cert:
   `iris cluster issue-cert -name iris-control-plane` (reference it in
   `cluster.client_cert/client_key`, plus `cluster.ca_cert: .../ca.crt`).
2. Register the node on the Cluster page and click **Enroll** — iris mints a
   single-use bootstrap token (bcrypt-hashed at rest, 1 h TTL) and shows the
   exact command once.
3. On the node: `iris cluster enroll -iris-url https://iris -name <node>
   -sans <agent ip/dns> -token <token> [-iris-ca <iris server ca>]` — the node
   generates its key locally (never leaves the host), sends a CSR, and writes
   `agent.crt/agent.key/ca.crt` (key 0600). The issued certificate's
   fingerprint is pinned on the node record; the token is consumed even if
   signing fails (issue a fresh one on retry).
4. Reference the files in the node's `agent:` section and start `iris agent`.

Offline alternative: `iris cluster issue-cert -server -sans ... -name <node>`
and copy the files manually.

### Redis Cluster / Sentinel client (required if Redis is clustered)

Both iris AND the kumod policy iris generates now use cluster-aware Redis
clients. This is a correctness requirement, not a nicety: a single-node client
cannot follow `MOVED`/`ASK` slot redirections, so against a Redis Cluster every
stream read fails (`MOVED <slot> <host>`). Configure it in one place:

- iris: `data.redis.addrs` (seed nodes) or `cluster: true`; Sentinel via
  `master_name`. Env: `IRIS_REDIS_ADDRS`, `IRIS_REDIS_CLUSTER`,
  `IRIS_REDIS_MASTER_NAME`.
- kumod: iris derives the generated policy's `redis.open`/
  `configure_redis_throttles` from the same config, emitting a seed-node array
  and `cluster = true` so kumod's log hook, suppression lookups, DSN/DMARC/FBL
  catchers, and shared throttles all use a cluster client.

### Redis hardening (required for multi-node)

Redis is the cluster bus (log streams, suppressions, shared throttles). On a
multi-node deployment:

- **Never expose Redis publicly**; bind it to the private cluster network.
- **Require AUTH + TLS** (`rediss://user:pass@host:6379` works everywhere a
  Redis URL is configured: `data.redis`, `kumomta.log_stream_redis_url`).
- **Scope kumod's ACL user** to what the policy actually does:

  ```
  user kumod on >SECRET ~iris.* ~supp:* &* -@all +xadd +exists +get +ping +hello
  user iris  on >SECRET2 ~* &* +@all -@admin
  ```

  (kumod XADDs onto `iris.*` streams, EXISTS/GET on `supp:*` suppression keys;
  the shared-throttle keyspace additionally needs `+@scripting +set +incr
  +expire` on `throttle*` / lease keys when `configure_redis_throttles` is on —
  verify against your KumoMTA version's throttle implementation.)

### kumo-proxy isolation (hard requirement)

kumo-proxy is unauthenticated. iris already refuses to render egress sources
pointing at public proxy IPs, but the network must enforce it too: bind
kumo-proxy to the private VLAN/WireGuard address only and firewall ingress to
cluster peers. Anyone who can reach a kumo-proxy can send traffic from your
reputation-bearing IPs.

### Certificate rotation

Leaf certs live 2 years. To rotate an agent: issue a new enroll token and
re-run `iris cluster enroll` (the pinned fingerprint updates), then restart
the agent. To rotate iris's client cert: `iris cluster issue-cert` again and
restart iris. CA rotation = new CA + re-enroll everything (plan a window).

## 10a. Migration: non-clustered production → clustered

The single-node install IS "a cluster of one." Every schema change is additive
with safe defaults and empty-registry ⇒ local transport, so the path is a
sequence of independent, individually-reversible steps. The existing node keeps
serving throughout; nothing routes to a new node until you assign a VMTA to it.

### Phase 0 — Upgrade in place (no cluster, safe rollback point)

1. Deploy the new iris binary. Migrations 0064–0067 apply automatically
   (`migrate_on_start`): they only ADD `mta_nodes`, enrollment tokens,
   `vmtas.node_id` (nullable, NULL = local node), `mail_records.node`,
   `mta_nodes.kumo_state` — all with defaults. **No behavior change**: with an
   empty registry the adapter uses the local file/reload transport exactly as
   before.
2. (Optional) Register the existing co-located node on the Cluster page with an
   EMPTY agent URL. That keeps it on the local transport (iris still writes the
   policy to its filesystem) but makes it visible, health-checked, and the
   receiving-node label populate. Leave its VMTAs' node ownership NULL.
3. Verify parity with the pre-upgrade behavior. Rollback = redeploy the old
   binary; the new columns are simply ignored.

### Phase 1 — Prepare shared infrastructure (still single-node)

Do this BEFORE a second node depends on it:

- **Redis** becomes the cluster bus (log stream, suppressions, shared
  throttles). Harden it now — AUTH + TLS + per-user ACLs (§6.4) — and make it
  HA (Sentinel or managed). iris/kumod just reconnect with the new `rediss://`
  URL; single-node keeps working.
- **Private network**: ensure the future nodes share it; plan to bind
  kumo-proxy, agents (:8447), Redis, and TSA to private addresses only.
- **Cluster CA**: `iris cluster init-ca`, issue iris's client cert, set
  `cluster.ca_dir` + `cluster.ca_cert/client_cert/client_key`. No effect on the
  running node; it just enables enrollment.
- **TSA** (if used): for a small cluster one shared daemon is enough; point
  `IRIS_TSA_URL` at a host all nodes can reach.

### Phase 2 — Bring up the new node (no traffic yet)

1. Provision the host: kumod, `proxy-server` (kumo-proxy) bound to its private
   IP and firewalled to cluster peers, and the iris binary (for `iris agent`).
2. Register the node in iris (name, `agent_url` https, `proxy_host/proxy_port`).
3. Enroll: issue a token on the Cluster page → run `iris cluster enroll` on the
   node → start `iris agent`. The node reports healthy.
4. Apply config. It fans out to both nodes; the policy is identical and still
   egress-only-on-node1, so this is a no-op for mail — it just proves the
   rollout path and pins the new node's applied checksum.

### Phase 3 — Shift egress to the new node

1. Assign VMTAs to the new node (`node_id` = node2) — either new egress IPs, or
   re-home an existing warmed IP.
   - **Re-homing a warmed IP is the delicate step.** Reputation follows the
     IP, not the host. Do it in a window: pause/drain that IP's queue on node1,
     move the IP (and its PTR/rDNS) onto node2 (or its proxy), set the VMTA's
     `node_id` to node2, then apply. Warmup state is preserved because the IP is
     unchanged.
2. Point the relevant routing rules at the node2 VMTAs (or add new rules).
   Existing traffic is unaffected until a rule targets a node2 VMTA.
3. Apply → rolling, health-gated. Mail routed to node2's VMTAs now egresses via
   node2's kumo-proxy, received-node logging intact (§2 of this doc).

### Phase 4 — Cluster-wide throttles + validation

- Crossing from 1 → 2 participating nodes makes the renderer emit
  `kumo.configure_redis_throttles` (an INIT-block change), so the next apply
  **restarts** kumod on each node (rolling). Expect and schedule that restart.
  Without it, two nodes would each apply the full rate limit (2× intended
  volume).
- Validate: Mail Logs shows both nodes' receptions with correct node
  attribution; per-node metrics (`node` label) populate; the cluster health
  page is green; deliverability steady.

### Rollback at any phase

The local node keeps working throughout. To back out a new node: repoint its
routing rules and reassign its VMTAs back to node1, apply, then set the node
`disabled` (or delete it). Only after egress no longer depends on node2 is it
safe to downgrade the iris binary.

### Prerequisite / known gap for this migration

- **Node-aware listener binds are supported** (migration 0068): pin a listener
  to a node so every node accepts submission on its own address from one
  identical policy. See open question 4.
- **Every node that OWNS VMTAs must run a kumo-proxy** for those VMTAs to be
  reachable from other nodes. A VMTA on a proxy-less node can only egress on
  its own node; if another node routes to it, that node cannot bind the source
  IP and the message defers. In a multi-submission cluster where any node may
  receive mail for any VMTA, give **every** egress-owning node a kumo-proxy.
  (Verified: with node1 proxy-less, a `promo` message submitted on node2 and
  routed to node1's VMTA deferred until node1 got its own proxy.)

## 10. Operator decisions & open questions

1. **DECIDED (2026-07-14): nodes are connected over an existing private
   network.** No iris-managed WireGuard mesh will be built. Deployment
   consequences: kumo-proxy, the agents (:8447), Redis, and the TSA daemon all
   bind to private-network addresses only; firewall ingress on those ports to
   cluster peers (plus iris for the agents). iris's node validation already
   enforces private-range proxy IPs (RFC1918/ULA/CGNAT), which matches this
   setup.
2. Redis HA flavor: Sentinel self-hosted vs managed?
3. Expected cluster size (2–5 vs dozens) — decides whether one TSA daemon is
   enough (it is for small N).
4. **DONE: node-aware listener binds implemented.** A listener may be pinned to
   a node (`listeners.node_id`, migration 0068) and renders inside an
   `if NODE_NAME == '<node>'` guard, so every node can accept submission on its
   own address from ONE byte-identical policy; an unpinned listener binds on
   every node. Proven end-to-end (both kumods bind only their own port from the
   same policy md5).

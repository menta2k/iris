# Routing rules

Routing rules decide **which egress pool** a message leaves through. They work in
two coordinated steps that run in the KumoMTA reception hook:

1. **Classify** the message into a *mailclass* (a label like `transactional` or
   `bulk`).
2. **Map** that mailclass to an egress pool ([VMTA](vmtas.md) or
   [group](vmta-groups.md)).

UI: **Outbound Config → Routing Rules** (`routing:read` / `routing:write`).

## Match types

A rule matches on one of:

| Match type | Matches against |
| ---------- | --------------- |
| `mailclass` | A header value (default header `X-Mailclass`, configurable per rule) |
| `recipient_email` | The exact envelope recipient |
| `recipient_domain` | The recipient's domain |
| `sender_ip` | The connecting client IP (CIDR or address) |

Each rule carries a **priority** (higher wins) and assigns a **mailclass**
and/or an **egress pool**.

## How classification works (reception hook)

1. Header/recipient rules run first, in priority order. The first match sets the
   `mailclass` meta.
2. If no rule classified the message, a **sender-IP** fallback runs: the
   connecting client's IP is matched against the `sender_ip` rules (highest
   priority first) to assign a mailclass.
3. The resulting mailclass is mapped to an **egress pool**; that pool is recorded
   as the `tenant` meta, which `get_queue_config` turns into the egress pool for
   delivery.

Mail that matches no rule routes to the `default` pool.

> **The `default` pool gotcha:** if you have no matching route and no explicit
> `default` pool, egress is *unpinned* — KumoMTA picks a source arbitrarily.
> Define a default route/pool so every outbound message has a deterministic
> source.

## Sender-IP classification

`sender_ip` rules are the bridge between an inbound submission and an outbound
mailclass: e.g. "mail submitted from `10.1.111.0/24` is `bulk`." They are matched
in descending priority and only consulted when no header/recipient rule already
classified the message.

## Related

- [VMTAs](vmtas.md) and [VMTA groups](vmta-groups.md) — the egress targets
- [Listeners](listeners.md) — where mail is accepted
- [KumoMTA config generation](kumomta-config.md) — the full reception pipeline

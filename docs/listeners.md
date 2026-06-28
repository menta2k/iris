# Listeners

A listener is an SMTP bind point KumoMTA opens — an IP/port plus its TLS and
relay settings. Iris models two **roles**:

| Role | Typical port | Purpose |
| ---- | ------------ | ------- |
| `inbound` | 25 | The MX bind point. Accepts mail for domains you host (and the bounce/FBL/DMARC/inbound-route domains). Relays only loopback. |
| `submission` | 587 | Authenticated/authorized outbound submission. Requires an explicit relay allowlist. |

UI: **Outbound Config → Listeners** (`outbound:read` / `outbound:write`).

## Fields

- **Name**, **IP address**, **Port**, **Hostname** (the EHLO/banner host).
- **TLS** — enable, with a certificate and key path (often an
  [ACME-issued](acme.md) cert).
- **Max message size.**
- **Role** — `inbound` or `submission` (defaults to `inbound`).
- **Relay hosts** — the CIDR/IP allowlist permitted to relay (submit outbound)
  through this listener.

## The relay model (3.0.0+)

The relay allowlist is **authoritative** — there is no implicit RFC-1918
fallback:

- **Loopback (`127.0.0.1/32`) is always permitted** on every listener, so on-box
  injection/submission works regardless of config.
- An **empty** relay list therefore means *loopback-only*: the listener accepts
  mail only for local/hosted domains and relays only for localhost. That is an
  **inbound / MX** listener.
- To authorize additional senders (a `:587` submission listener), **list their
  CIDRs explicitly**.

Role and relay list are kept consistent at validation time:

- An `inbound` listener **must not** list relay hosts.
- A `submission` listener **must** list at least one relay host beyond loopback.

> **Gotcha:** an inbound-only listener with no route/relay match produces
> unpinned egress for any mail it does accept for relay. Pair listeners with
> [routing rules](routing-rules.md) and [VMTAs](vmtas.md) so accepted mail has a
> defined egress path.

## How it renders

Each active listener becomes a `kumo.start_esmtp_listener { … }` call inside
`kumo.on('init')`, with `relay_hosts` = loopback + the configured CIDRs.

> Because listeners live in the **init block**, changing them requires a KumoMTA
> **restart**, not just a reload. See [KumoMTA config](kumomta-config.md).

## Related

- [VMTAs](vmtas.md) and [VMTA groups](vmta-groups.md) — egress identity
- [Routing rules](routing-rules.md) — where accepted mail goes

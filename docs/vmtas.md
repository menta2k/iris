# VMTAs (egress sources)

A **VMTA** is a virtual MTA — an outbound egress source. In KumoMTA terms it is
an *egress source*: the IP address mail leaves from and the EHLO name it presents
to remote servers. Reputation is built per source, so VMTAs are the unit you
scale and isolate sending across.

UI: **Outbound Config → VMTAs** (`vmta:read` / `vmta:write`).

## Fields

- **Name** — referenced by routing and groups.
- **IP address** — the source address for outbound connections.
- **EHLO name** — the hostname presented in EHLO (and typically the PTR/rDNS
  target for that IP). A correct EHLO/rDNS pairing matters for deliverability.
- **Max connections** — per-source connection limit, applied via the egress path
  config.
- **Status** — `active`, `draining` (finish in-flight, accept no new), or
  disabled.

> Since the decoupling in 3.x, a VMTA **owns its own IP and EHLO** independently
> of any listener — the two are no longer tied together.

## How it renders

Each active VMTA becomes a KumoMTA **egress source** (with its EHLO and source
address), and its `max_connections` is enforced in `get_egress_path_config`.
A VMTA must belong to a [pool](vmta-groups.md) (a singleton pool per VMTA is
created automatically) to receive traffic; [routing rules](routing-rules.md)
select the pool.

## Default EHLO

If a VMTA does not set its own EHLO, the deployment-wide
`egress_ehlo_domain` global setting is used as the default.

## Related

- [VMTA groups](vmta-groups.md) — pooling and weighting
- [Routing rules](routing-rules.md) — selecting an egress pool
- [Listeners](listeners.md)

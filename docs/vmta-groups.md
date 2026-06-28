# VMTA groups (egress pools)

A VMTA group is a **weighted pool** of [VMTAs](vmtas.md). Routing selects a pool;
KumoMTA then distributes outbound mail across the pool's members in proportion to
their weights. Use groups to spread a mail stream across several IPs, or to
warm up new IPs by giving them small weights.

UI: **Outbound Config → VMTA Groups** (`vmta:read` / `vmta:write`).

## Fields

- **Name** — referenced by [routing rules](routing-rules.md).
- **Members** — one or more VMTAs, each with an integer **weight**.
- **Status.**

The editor resolves member ids to VMTA names and shows each member's **effective
share** as a percentage of the pool (e.g. weights 3 and 1 render as 75% / 25%),
and prevents selecting the same VMTA twice.

## How it renders

Each active group becomes a KumoMTA egress **pool** whose entries are the member
sources with their weights. A per-VMTA singleton pool also exists, so a routing
rule can target either a single VMTA or a multi-member group.

## Routing to a pool

Egress selection happens in routing: a rule classifies a message into a
**mailclass**, and the mailclass maps to a pool (group or singleton). Mail with
no matching route falls back to the `default` pool — keep an explicit default to
avoid unpinned egress. See [routing rules](routing-rules.md).

## Related

- [VMTAs](vmtas.md)
- [Routing rules](routing-rules.md)

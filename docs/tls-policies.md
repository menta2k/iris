# Require TLS

A require-TLS policy forces KumoMTA to deliver to a destination domain **only
over TLS** — if the receiving server offers no STARTTLS, the delivery fails
(and is logged) rather than falling back to cleartext. Use it for domains where
you have a contractual or compliance requirement to encrypt in transit.

UI: **Domain Safety → Require TLS** (`domain-safety:read` / `domain-safety:write`).

## Fields

- **Domain** — the destination (recipient) domain the policy applies to.
- **Mode** —
  - `required` — STARTTLS with certificate verification.
  - `required_insecure` — STARTTLS required, but certificate validation relaxed
    (use only when a destination has a broken/self-signed cert you must tolerate).
- **Status.**

## How it renders

Active policies populate a `REQUIRE_TLS_DOMAINS` table keyed by destination
domain. In `get_egress_path_config`, when the delivery's domain matches, KumoMTA's
`enable_tls` is set accordingly, so it refuses to send in cleartext to that
domain.

The same egress-path mechanism is reused to honor the per-route TLS mode of a
**forward** [inbound route](inbound-routing.md) (relay to a smarthost with
`required` TLS).

## Note

This controls **outbound delivery** TLS to remote domains. TLS for *receiving*
mail (your listeners' certificates) is configured per
[listener](listeners.md), often with an [ACME-issued](acme.md) certificate.

## Related

- [Listeners](listeners.md) — inbound TLS
- [ACME / TLS certificates](acme.md)

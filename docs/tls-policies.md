# TLS Policy

A per-destination-domain TLS policy controls how KumoMTA negotiates TLS when
delivering to a remote domain. It can **require** TLS (fail rather than send in
cleartext) for domains where you must encrypt in transit, or **relax/disable**
it for receivers whose broken or legacy certificate kumod cannot negotiate.

UI: **Domain Safety → TLS Policy** (`domain-safety:read` / `domain-safety:write`).

## Default (no policy)

Domains without a policy use **opportunistic** TLS: kumod encrypts if the
receiver offers STARTTLS but does **not** hard-fail on certificate verification
(`OpportunisticInsecure`). This is the right default for port-25 delivery —
many large receivers serve legacy or incomplete chains that a strict verifier
would turn into silent deferrals.

## Fields

- **Domain** — the destination (recipient) domain the policy applies to.
- **Mode** — maps to KumoMTA's `enable_tls`:

  | Mode | enable_tls | Behavior |
  | --- | --- | --- |
  | `required` | `Required` | STARTTLS **and** a valid certificate; fail if unavailable |
  | `required_insecure` | `RequiredInsecure` | STARTTLS required, certificate validation skipped |
  | `opportunistic_insecure` | `OpportunisticInsecure` | Try TLS (skip cert), fall back to cleartext — the default, made explicit |
  | `disabled` | `Disabled` | Never attempt STARTTLS; deliver in cleartext |
- **Status.**

## When to disable TLS

Some receivers present a certificate kumod's TLS stack cannot even **parse** —
e.g. a legacy X.509 v1 certificate, which surfaces in the mail log as:

```
invalid peer certificate: UnsupportedCertVersion
… All failures are related to OpportunisticInsecure STARTTLS.
Consider setting enable_tls=Disabled for this site.
```

Because the failure happens during certificate parsing (before the
"insecure is OK" logic), even the opportunistic default hard-fails the handshake
and the message **never delivers** — it defers and eventually bounces
(`Expiration`, `max_age`). Setting the domain to **`disabled`** makes kumod skip
STARTTLS and deliver in cleartext, so the mail gets through. `BadSignature` on
the peer certificate is another symptom of the same class.

## How it renders

Active policies populate a `REQUIRE_TLS_DOMAINS` table keyed by destination
domain. In `get_egress_path_config`, when the delivery's domain matches, KumoMTA's
`enable_tls` is set to the mode's value (else the opportunistic default applies).

The same egress-path mechanism honors the per-route TLS mode of a **forward**
[inbound route](inbound-routing.md) (relay to a smarthost with `required` or
`disabled` TLS).

## Note

This controls **outbound delivery** TLS to remote domains. TLS for *receiving*
mail (your listeners' certificates) is configured per
[listener](listeners.md), often with an [ACME-issued](acme.md) certificate.

## Related

- [Mail logs](mail-logs.md) — where TLS handshake failures surface
- [Listeners](listeners.md) — inbound TLS
- [ACME / TLS certificates](acme.md)

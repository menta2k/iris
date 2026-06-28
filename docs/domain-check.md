# Domain bounce readiness

Before a sending domain will deliver well (and before its bounces/DSNs can be
captured), its DNS needs to be set up correctly. The Domain Bounce Readiness tool
checks a domain's live DNS for the records that matter.

UI: **KumoMTA → Domain Bounce Readiness** (`service:control`).

## What it checks

For a given domain, Iris performs **live DNS lookups** and reports on:

- **MX** — does the domain (or its bounce/feedback subdomain) point at your
  KumoMTA, so async bounces and complaints are delivered to you?
- **SPF** — is your sending source authorized?
- **DKIM** — is the public key for your configured [selector](dkim.md) published
  at `<selector>._domainkey.<domain>` and does it match the stored key?

The result is a readiness summary so you can fix DNS before relying on
[bounce handling](bounce-handling.md), [feedback loops](feedback-loops.md), or
[DMARC](dmarc.md) capture.

## Related

- [DKIM](dkim.md)
- [Bounce handling](bounce-handling.md)
- [Diagnostics](diagnostics.md) — sender diagnose & RBL checks

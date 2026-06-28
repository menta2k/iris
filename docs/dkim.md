# DKIM signing

Iris manages DKIM keys per domain and makes KumoMTA sign outbound mail with them.
DKIM lets receivers verify a message was authorized by your domain and was not
altered in transit — a deliverability prerequisite alongside SPF and DMARC.

UI: **Domain Safety → DKIM Domains** (`dkim:read` / `dkim:write`).

## What you configure

- **Domain** — the signing domain (the `d=` tag).
- **Selector** — the key selector (the `s=` tag); the public key is published at
  `<selector>._domainkey.<domain>`.
- **Private key** — Iris can **generate** a key pair for you, or you can import a
  PEM private key (stored as a reference, never echoed back or written to the
  [audit log](audit-log.md)).
- **Status** — `ready`, `disabled`, or `needs_attention`.

For each domain Iris can show the **public DNS record** to publish (the TXT
record at `<selector>._domainkey.<domain>`).

## How signing works

The generated policy builds a DKIM signer table from the active `ready` keys and
calls it from the reception hook (`iris_dkim_sign`) — KumoMTA signs **on
reception**, since there is no separate client-sending hook. Only domains with a
usable key sign; others pass through unsigned.

## Relationship to other features

- The same keys back **FBL provenance verification**: an inbound ARF complaint is
  trusted in part by checking that the embedded original message carries a DKIM
  signature that verifies against *our* published key. See
  [feedback loops](feedback-loops.md).
- DKIM domains also seed the **hosted-domains** set used to scope inbound
  [rspamd scanning](rspamd.md) when no explicit hosted set is configured.

## Related

- [Domain bounce readiness](domain-check.md) — verify MX/SPF/DKIM DNS live
- [Feedback loops](feedback-loops.md)

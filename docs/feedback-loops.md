# Feedback loops (FBL / ARF)

Mailbox providers (Gmail, Yahoo, Outlook, …) offer **feedback loops**: when a
recipient marks your mail as spam, the provider emails you a complaint in
**ARF** format (RFC 5965). Iris ingests these complaints, verifies they really
concern mail you sent, and suppresses the complainant.

UI: **KumoMTA → Feedback Loops** (`service:control`) to manage endpoints;
**Operations → Feedback** (`operations:read`) to view complaints.

## Endpoints and the two states

Each FBL **endpoint** enrolls one domain:

| State | Behavior in the policy |
| ----- | ---------------------- |
| `approved` | KumoMTA parses ARF reports at the domain (`log_arf`) and emits a `Feedback` log record, which the log hook streams to Iris for auto-suppression. |
| `awaiting_approval` | The domain is relayed, and mail arriving at the feedback address is **forwarded** to a human mailbox (so you can read the provider's enrollment-confirmation email) before approving. |

The forward re-injects a new locally-originated message (rewriting the sender to
a local address at the feedback domain so it passes SPF), rather than relaying
the inbound message — which would be rejected as open relaying.

## Provenance verification (anti-poisoning)

A complaint should only suppress a recipient if it genuinely concerns mail
**we sent**. Iris verifies provenance in layers (the `log-stream` worker, via
`VerifyFeedback`), and records which method succeeded:

1. **Supplemental trace** — KumoMTA injects an `X-KumoRef` trace header on
   outbound mail (base64 JSON carrying the recipient); if the complaint's
   embedded original carries it, the recipient is proven.
2. **Send log** — the embedded original's `Message-ID` is looked up in our mail
   log; a match proves we sent it.
3. **DKIM by us** — the embedded original is DKIM-verified **offline against our
   own published keys**; a valid signature by our key proves authorship.

ARF *structure* validation is free: KumoMTA only emits a `Feedback` record for a
well-formed RFC 5965 report, so Iris does not re-parse it.

## Rollout: configurable, default permissive

The `fbl_require_verification` global setting gates suppression on proof:

- **Off (default):** every complaint suppresses the complainant (prior behavior),
  but the verification method is still recorded for visibility.
- **On:** only complaints with proven provenance auto-suppress.

Each feedback record stores `verified` and the `verification` method, surfaced as
a badge on the Feedback page.

## A note on domains

Keep the FBL domain **separate** from the DMARC report domain — a single domain
serving both collides in `get_listener_domain` precedence and can swallow one of
them. Use a dedicated subdomain for feedback.

## Related

- [Suppressions](suppressions.md)
- [DKIM](dkim.md) — the keys used for the DKIM provenance check
- [DMARC reports](dmarc.md)

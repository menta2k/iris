# Suppressions

The suppression list prevents Iris/KumoMTA from sending to addresses or domains
you should not contact — hard-bounced recipients, complainers, and manual blocks.
Suppression is enforced **at SMTP time** in the reception hook: a suppressed
recipient is rejected before the message is queued.

UI: **Domain Safety → Suppressions** (`suppression:read` / `suppression:write`).

## Entry model

- **Type** — `email` (a single recipient) or `domain` (a whole domain).
- **Value** — the normalized address or domain.
- **Reason / source** — why it was added (manual, `fbl`, bounce, etc.).
- **Status** — `active`, `disabled`, or `expired`.
- **Expiry** — derived from the TTL (empty = permanent).

Addresses are sanitized on entry (control/zero-width characters stripped) so a
copy-pasted address with hidden characters still matches.

## Where it lives: Redis, mirrored to Postgres

For the policy to check suppression at SMTP time it needs a fast lookup, so the
**live suppression list lives in Redis** (a write-through cache). The database is
the durable mirror; on startup Iris backfills Redis from the active DB entries so
a restart or flush stays consistent. The list is **not** rendered inline into the
Lua policy — the policy queries Redis.

## TTL

The `suppression_ttl` global setting sets the default lifetime applied to new
suppression records (KumoMTA/Go duration form, e.g. `720h`, `30d`; empty =
permanent). It is applied both as the Redis key TTL and mirrored to the DB
`expires_at`, so entries age out automatically.

## Automatic suppression

Other subsystems add entries automatically:

- **Hard bounces** auto-suppress when `auto_suppress_hard_bounces` is on; soft
  bounces suppress after `soft_bounce_threshold` occurrences. See
  [bounce handling](bounce-handling.md).
- **FBL complaints** suppress the complainant (subject to provenance
  verification). See [feedback loops](feedback-loops.md).

## Related

- [Bounce handling](bounce-handling.md)
- [Feedback loops](feedback-loops.md)

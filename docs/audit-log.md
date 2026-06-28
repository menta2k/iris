# Audit log

Iris records an audit entry for sensitive operations — configuration changes,
applies, user/role changes, and other state mutations — so you can answer
"who changed what, when, and did it succeed."

UI: **Security & Audit → Audit Log** (permission `audit:read`).

## What is recorded

Each entry captures:

- **Operation** — e.g. `vmta.create`, `routing.update`, `config.apply`,
  `inbound_route.delete`, `user.update`.
- **Target** — the entity type and id/name affected.
- **Actor** — the authenticated user that performed it.
- **Outcome** — success or failure.
- **Summary** — a small structured payload of the salient fields.
- **Timestamp.**

Secrets are deliberately omitted from summaries (for example, DKIM private keys
and webhook secret references are never written to the audit payload).

## Where it comes from

Use cases call an `Auditor` after performing a change; both successes and
failures are logged, so a denied or failed attempt still leaves a trace. Entries
are stored in the `audit_entries` table and listed newest-first with pagination.

## Related

- [Authorization](authorization.md)
- [KumoMTA config generation](kumomta-config.md) — applies are audited

# Authorization (RBAC) & user management

Every protected API operation checks a **fine-grained permission**. Permissions
are granted to **roles**, and roles are assigned to **users**. A user's effective
permission set is the union of its roles' permissions.

UI: **Security & Audit → Users** (`user:read` / `user:write`) and
**MFA & Permissions** (`security:read`).

## Permissions

| Area | Permissions |
| ---- | ----------- |
| Outbound | `vmta:read`, `vmta:write`, `routing:read`, `routing:write` |
| Mail operations | `mail:read`, `queue:read`, `queue:control`, `service:control`, `worker-logs:read` |
| Domain & recipient safety | `dkim:read`, `dkim:write`, `suppression:read`, `suppression:write` |
| Inbound automation | `webhook:read`, `webhook:write`, `rspamd:read` |
| Inbox monitoring | `monitoring:read`, `monitoring:write` ([inbox monitoring](inbox-monitoring.md)) |
| Security | `user:read`, `user:write`, `audit:read` |
| Settings & dashboard | `settings:read`, `settings:write`, `dashboard:read` |
| Wildcard | `*` (all permissions; reserved for `owner`) |

> The `webhook:read` / `webhook:write` permissions gate **inbound routes**
> (the inbound-routing model that replaced standalone webhooks). The names are
> retained for compatibility.

## Built-in roles

Seeded by a migration, so role assignment works on a fresh database:

| Role | Intent | Grants |
| ---- | ------ | ------ |
| `owner` | Full control | `*` |
| `operator` | Day-to-day configuration & operations | outbound r/w, mail/queue/service, worker logs, DKIM r/w, suppression r/w, inbound r/w, rspamd read, dashboard, settings r/w, monitoring r/w |
| `security_admin` | User & audit administration | user r/w, audit read, dashboard, mail/queue read, worker logs |
| `viewer` | Read-only across the product | the `*:read` permissions |

Custom roles can be created with any subset of permissions.

## How checks work

Each use case calls `RequirePermission(ctx, …)` before doing work. The wildcard
`*` (owner) satisfies any check. A missing permission returns
`403 PERMISSION_DENIED`. Authorization is enforced server-side; the SPA also
hides nav items the user lacks permission for (the `permission` field on each
nav entry) but that is presentation only.

## Users

Manage users under **Security & Audit → Users**:

- Create users (active/disabled), assign one or more roles.
- Reset is handled via password change ([authentication](authentication.md)).
- MFA enrollment status is visible under **MFA & Permissions**.

The first owner is seeded from the environment on an empty database — see
[Bootstrapping the first admin](authentication.md#bootstrapping-the-first-admin).

## Related

- [Authentication](authentication.md)
- [Audit log](audit-log.md)

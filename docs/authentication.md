# Authentication

Iris uses password login with optional TOTP multi-factor authentication. Access
is gated by a signed, stateless session token (HMAC-SHA256 over the deployment's
`auth.session_token_secret`).

## Configuration

```yaml
auth:
  session_ttl: 12h
  session_token_secret: "<long-random-secret>"  # required unless dev_bypass
  mfa_required: true        # force every user through MFA
  dev_bypass: false         # true = no auth at all (local dev only)
```

Environment overrides: `IRIS_SESSION_SECRET`, `IRIS_AUTH_DEV_BYPASS`.

When `dev_bypass` is `false`, the backend refuses to start without a session
secret — it will not sign tokens with an empty key.

> `dev_bypass: true` disables authentication entirely (injects a full-permission
> identity). Only use it locally, or behind a trusted reverse proxy / private
> network.

## Bootstrapping the first admin

On an empty user table, set both of these to seed an initial owner account:

```bash
IRIS_BOOTSTRAP_ADMIN_EMAIL=admin@example.com
IRIS_BOOTSTRAP_ADMIN_PASSWORD=<long-password>
```

The account is created active with the `owner` role. It is a no-op once any user
exists. After the first login you can remove the variables.

The built-in roles (`owner`, `operator`, `security_admin`, `viewer`) are seeded
by a migration, so role assignment works on a fresh database.

## Login flow

```
POST /v1/auth:login            { email, password }   -> { token, status, user, permissions }
POST /v1/auth:verify-mfa       { code }              -> { token, status, user, permissions }
GET  /v1/auth:me                                     -> { user, permissions }
POST /v1/auth:change-password  { current_password, new_password }
POST /v1/auth:logout
```

`status` drives the next step:

- `authenticated` — the token is fully usable.
- `mfa_required` — credentials were valid; submit a TOTP code to `auth:verify-mfa`.
- `mfa_enrollment_required` — the user must enroll first via `mfa:enroll` /
  `mfa:confirm` (the confirm response returns an upgraded token).

Send the token as `Authorization: Bearer <token>` on every request. A
partially-authenticated (pre-MFA) token may only call the MFA-completion
endpoints (`verify-mfa`, `mfa:enroll`, `mfa:confirm`), `auth:me`, and
`auth:logout`; everything else returns `403 MFA_REQUIRED` until MFA is cleared.

## MFA (TOTP)

Standard RFC 6238 TOTP (SHA1 / 6 digits / 30s). `mfa:enroll` returns a secret and
an `otpauth://` URI to add to an authenticator app; `mfa:confirm` validates a
code to activate it. The SPA renders the enrollment and verification screens at
`/mfa`.

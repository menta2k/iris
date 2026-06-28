# Inbound routing

Inbound routes decide what happens to mail that arrives **for a domain or address
you host**. A single **InboundRoute** matches the recipient and dispatches the
message to one of three native KumoMTA queue protocols — no extra workers or
streams are involved; everything is rendered into the policy and `kumod` does the
delivery.

UI: **Inbound Automation → Inbound Routes** (`webhook:read` / `webhook:write`).

## The model

| Field | Meaning |
| ----- | ------- |
| **Match type** | `recipient_email` (exact address) or `recipient_domain` |
| **Match value** | the address or domain |
| **Action** | `maildir`, `forward`, or `webhook` |
| **Priority** | tie-breaker when several routes could match |
| **Spam scan** | `default` / `off` / `tag` / `enforce` — per-route rspamd ([see below](#per-route-spam-scanning)) |
| **Status** | `active` / `disabled` |
| _action params_ | maildir path / forward host+port+TLS / webhook URL+secret |

An exact **email match outranks a domain match**; a partial unique index keeps at
most one active route per recipient.

## The three actions

### maildir

KumoMTA writes the message to a **Maildir on disk** using its native
`maildir_path` protocol. Without an explicit path, the message lands under the
deployment-wide base (`inbound_maildir_base_path` global setting) at
`<base>/<domain>/<local-part>` — one mailbox per recipient, via KumoMTA's path
templating. Set a per-route path to override.

### forward (smarthost relay)

KumoMTA relays the message to a **pinned smarthost** (`host:port`) using the
native `smtp` protocol with a fixed `mx_list`, bypassing MX resolution. The
envelope recipient is preserved. A per-route **TLS mode** (`none` /
`opportunistic` / `required`) is honored on the egress path (`required` fails the
delivery if the smarthost offers no STARTTLS). This is the classic "relay
everything for this domain to my internal MTA" pattern.

### webhook

KumoMTA POSTs the **raw RFC822 message** to an HTTPS endpoint
(`make.webhook_post`), with `Content-Type: message/rfc822`, `X-Iris-Recipient` /
`X-Iris-Message-Id` headers, and — when a secret reference is set — an
`X-Iris-Signature` HMAC-SHA256 of the body. Delivery happens in-policy; there is
no separate fan-out worker.

> Inbound routes replaced the former standalone "webhook rules" feature. The
> webhook action subsumes it.

## How it renders

The renderer emits per-recipient dispatch tables and uses the same pattern the
DSN/DMARC pipelines use:

1. `get_listener_domain` relays every inbound-route domain (`ROUTE_DOMAINS`).
2. The reception hook looks up the recipient (`ROUTE_BY_EMAIL` then
   `ROUTE_BY_DOMAIN`), optionally scans with rspamd, then tags the message's
   `queue` for its action and returns.
3. `get_queue_config` turns that synthetic queue into the right protocol: the
   webhook poster, a `maildir_path` destination, or an `smtp` smarthost.

A recipient that hits a relayed domain but matches no route is **rejected** (so
the sending MTA bounces it) rather than relayed to the domain's real MX.

## Per-route spam scanning

Each route's `spam_scan` overrides the deployment-wide rspamd mode for its own
captured mail:

| Mode | Behavior |
| ---- | -------- |
| `default` | Follow the global rspamd mode |
| `off` | Never scan this route |
| `tag` | Scan and add `X-Spam` headers, never reject |
| `enforce` | Scan and reject a spam verdict at SMTP time |

Scanning happens **before** the message is stored/forwarded/posted, so the
verdict headers ride along (maildir/forward) and a rejected message never lands.
Per-route `tag`/`enforce` works even when the global mode is `off`, as long as an
rspamd URL is configured. See [rspamd](rspamd.md).

## Related

- [Rspamd spam filtering](rspamd.md)
- [Listeners](listeners.md) — the `:25` MX that accepts inbound mail
- [KumoMTA config generation](kumomta-config.md) — the reception pipeline

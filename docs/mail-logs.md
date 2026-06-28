# Mail logs

The Mail Logs view is built **from KumoMTA's own structured logs**, not from
manual inserts. The generated policy's `log_hook` streams every log record into
Redis; the `log-stream` worker ingests them into the `mail_records` hypertable.

UI: **Operations → Mail Logs** (`operations:read`).

## Record types

KumoMTA emits these record types, which Iris maps to a mail status:

| KumoMTA record | Meaning |
| -------------- | ------- |
| `Reception` | A message was accepted |
| `Delivery` | Delivered to the next hop |
| `Bounce` | Permanent failure (a DSN/rejection) |
| `TransientFailure` | Temporary failure (will retry) |
| `Feedback` | An ARF complaint ([feedback loops](feedback-loops.md)) |
| `AdminBounce` | An operator-initiated bounce |
| `Expiration` | The message exceeded `max_age` and was abandoned |

Iris also synthesizes a `Suppressed` record when the reception hook rejects a
suppressed recipient, so suppressed mail still appears in the log.

## What a record carries

Message id, event time, mailclass, sender, the original `From` header, recipient
and domain, egress source (present on Delivery/Bounce), status, SMTP
status/diagnostic, and — for bounces — a classification category. Records are
keyed by **message id**, so the UI can reconstruct a single message's timeline
across receptions, transient failures, and the final delivery/bounce.

> The envelope sender is VERP-rewritten at reception, so the **From header** is
> the place the original sender survives — it is included in the log hook's
> header allow-list for that reason.

## How streaming is configured

The policy log hook needs a Redis URL reachable **from KumoMTA**. Set
`log_stream_redis_url` (global setting or `kumomta.log_stream_redis_url`); empty
derives it from the backend's own Redis address, which is wrong when KumoMTA and
the backend reach Redis at different addresses (e.g. containers). The stream name
is `iris.mail.events`.

## Related

- [Bounce handling](bounce-handling.md)
- [Dashboard & metrics](dashboard.md)
- [Architecture](architecture.md) — the event bus

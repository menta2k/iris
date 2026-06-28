# Bounce handling (DSN, VERP, classification)

When a remote server rejects mail, it may reject **synchronously** (at SMTP time)
or **asynchronously** by emailing back a Delivery Status Notification (DSN, aka a
bounce) later. Iris captures both, classifies them, correlates async bounces to
the original message via VERP, and auto-suppresses recipients.

UI: **Operations → Bounces** (`operations:read`). Settings live in
**Global Settings**.

## The bounce domain

Set a **bounce domain** (`bounce_domain` global setting), e.g.
`bounces.example.com`, and point its MX at your KumoMTA. The generated policy:

- accepts inbound mail for that domain (`get_listener_domain`), and
- in the reception hook, routes mail addressed there to a **DSN tracker** queue
  that XADDs the raw message onto the `iris.dsn.events` Redis stream.

The **`dsn` worker** consumes the stream, parses the DSN, classifies it, and
(when warranted) suppresses the recipient.

## VERP — correlating async bounces

To know *which* message an async DSN is about, the envelope return-path is
rewritten per message using **VERP**: the reception hook sets the sender to
`b+<hmac>.<message-id>@<bounce-domain>`. The HMAC is signed with a key derived
from the session secret, shared by the policy and the `dsn` worker, so the worker
can verify the token and recover the message id without separate storage.

> VERP is rendered only when both a bounce domain **and** a signing secret are
> present. It is applied at **reception** (KumoMTA has no client-sending hook).

## Classification

When KumoMTA's bounce-classifier rules file is available
(`IRIS_BOUNCE_CLASSIFIER_FILE`, default the IANA rules), bounce log records carry
a **category** (e.g. invalid recipient, mailbox full, policy). Categories drive
smarter suppression and reporting.

## Auto-suppression

- **Hard bounces** auto-suppress the recipient when `auto_suppress_hard_bounces`
  is enabled.
- **Soft bounces** suppress after `soft_bounce_threshold` occurrences.

Suppressed recipients are then rejected at SMTP time on future sends. See
[suppressions](suppressions.md).

## Related

- [Suppressions](suppressions.md)
- [Mail logs](mail-logs.md) — bounces also appear as log records
- [Feedback loops](feedback-loops.md) — the complaint counterpart

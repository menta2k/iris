# Worker errors

Iris runs several background workers (log ingestion, DSN, DMARC, rspamd, service
control, ACME). When one of them logs a warning or error — a malformed record, a
Redis hiccup, a parse failure — that event is captured into a **generic
worker-error log** so failures in the background pipeline are visible in the UI
instead of only in stdout.

UI: **Operations → Worker Errors** (`worker-logs:read`).

## How it works

The base structured logger is wrapped with a tee handler (`internal/errlog`):
every `Warn` and `Error` a worker emits is mirrored into the `worker_error_logs`
store, in addition to going to stdout. Each entry records the worker name, level,
message, and structured context, with a timestamp.

The supervisor that runs the workers (`startWorker`) keeps the **plain** stdout
logger, so a failure while writing to the error-log sink can never recurse back
through the database handler. The `errlog-flush` worker drains the sink.

## What you'll see

Typical entries: "logstream: suppressed xadd failed", "rspamd: request failed",
"dmarc: parse error", "ingest rspamd result". Use it to spot a stuck pipeline
(e.g. Redis unreachable, a malformed report) before it shows up as missing data
in the Logs or DMARC views.

## Related

- [Architecture](architecture.md) — the worker set
- [Mail logs](mail-logs.md)

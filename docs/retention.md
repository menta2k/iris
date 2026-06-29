# Retention & cleanup

At volume (millions of messages/day) the event tables — `mail_records` above all —
grow fast. Iris cleans them up by **dropping whole TimescaleDB chunks**, which
returns disk to the operating system *immediately*. This page explains why that
avoids the classic "deleted rows never free disk" problem and how to configure
retention per table.

UI: **Operations → Retention** (gated by `service:control`).

## Why chunk-drop, not `DELETE`

Row-level `DELETE` is the wrong tool for log cleanup, on **both** MySQL/InnoDB and
PostgreSQL:

- **InnoDB:** `DELETE` frees space inside the tablespace but the `.ibd` file never
  shrinks; reclaiming OS disk needs `OPTIMIZE TABLE` (a full rebuild, ~2× disk,
  heavy lock).
- **PostgreSQL:** `DELETE` leaves *dead tuples*. Plain `VACUUM` makes the space
  reusable by the same table but does **not** return it to the OS. `VACUUM FULL`
  does, but takes an `ACCESS EXCLUSIVE` lock (blocks everything), needs ~2× disk,
  and is slow on large tables.

The Iris event tables are **TimescaleDB hypertables** — transparently partitioned
into **chunks**, each a real child table with its own files on disk. Retention
drops whole chunks:

- A chunk drop is a `DROP TABLE` of the child → its heap **and** indexes **and**
  TOAST are unlinked and the **space is returned to the OS instantly**.
- No `VACUUM`, no bloat, no 2× space, no long lock, almost no WAL.

So the "disk never frees" problem does not occur, *as long as cleanup is
chunk-based* (which is what Iris does). Iris never issues row-level `DELETE` for
retention.

## What you configure (per table)

Each managed event hypertable has its own policy on the Retention page:

| Field | Meaning |
| ----- | ------- |
| **Keep (days)** | Drop chunks older than this. **0 = keep forever.** |
| **Compress after (days)** | Compress chunks older than this (~90% smaller) before they are dropped. 0 = no compression. Must be **less than** Keep. |
| **Enabled** | Whether the daily worker runs this policy. |

Managed tables and their defaults:

| Table | Keep | Compress after |
| ----- | ---- | -------------- |
| `mail_records` (Mail logs) | 90d | 7d |
| `bounce_records` | 90d | 7d |
| `feedback_reports` | 180d | 14d |
| `rspamd_filter_results` | 30d | 7d |
| `queue_snapshots` | 14d | — |
| `service_control_requests` | 90d | — |
| `audit_entries` (Audit log) | **forever** | — |

`audit_entries` defaults to keep-forever so a compliance setup is never surprised
by automatic deletion — opt in explicitly if you want it trimmed.

## Compression

The recommended shape is **recent → compressed → dropped**:

- Newest chunks stay uncompressed (fast writes/queries).
- Chunks older than *Compress after* are converted to TimescaleDB's columnar
  compression — typically **90–95% smaller**, so you keep far more history per GB.
- Chunks older than *Keep* are dropped.

Compressed chunks are append-friendly, which fits these append-only logs. The
Retention page shows the compressed footprint and compressed-chunk count so you
can see the savings.

## How it runs

A background **retention worker** runs daily and on demand:

- **Daily:** for each enabled policy it compresses newly-eligible chunks, then
  drops chunks past the Keep window, recording the run (chunks compressed/dropped,
  bytes before/after).
- **Run now:** the page has per-table and "Run all" buttons that enqueue an
  immediate pass (via the `iris.retention.commands` stream).

Each run's result — including disk **freed** — is shown on the page; failures
also surface in the [Worker Errors](worker-errors.md) log.

## Sizing guidance

- **Measure rows, not messages.** Each message produces several `mail_records`
  rows (Reception → Delivery/Bounce/TransientFailure…), so 3M messages/day can be
  10–20M rows/day. Size your windows against that.
- **Chunk interval.** New chunks are created at **1-day** intervals (set by
  migration `0034`) so retention is granular and chunks stay a manageable size.
- **Indexes dominate at volume.** `mail_records` carries several indexes; they can
  cost more disk and write-IO than the heap. If disk is tight, review whether all
  are needed for your queries.

## Plain PostgreSQL (no TimescaleDB)

Chunk-based retention requires the `timescaledb` extension. On a plain-PostgreSQL
deployment the event tables are ordinary tables with no chunks: the Retention page
marks them **"TimescaleDB not enabled"** and the worker safely no-ops for them
(it never falls back to bloating `DELETE`). The bundled deployment uses the
TimescaleDB image, so this is a guard rather than the common case.

## Long-term dashboards (note)

The `*_stats_1h` views compute over the raw tables, so once raw chunks are dropped
their history is gone. If you need charts that outlive raw retention, the planned
follow-up is **materialized continuous aggregates** (hourly/daily rollups kept for
a year or more) so you can shorten raw retention aggressively while keeping
long-range history cheaply.

## Related

- [Mail logs](mail-logs.md)
- [Worker errors](worker-errors.md)
- [Architecture](architecture.md) — hypertables and the event bus

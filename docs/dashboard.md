# Dashboard & metrics

The dashboard is the landing view: a summary of current state plus mail-flow
time-series.

UI: **Overview → Dashboard** (`dashboard:read`).

## Summary

The summary cards are computed from Iris's own data (the `mail_records` and
related hypertables, with TimescaleDB continuous aggregates for efficiency):
recent receptions, deliveries, bounces, deferrals, and queue health at a glance.

## Mail-flow time-series (Prometheus)

The mail-flow chart is backed by **Prometheus**. Set the `prometheus_url` global
setting to your Prometheus base URL; the dashboard endpoint queries it for
delivery/bounce/deferral/reception rates over a selectable range. When no
Prometheus URL is configured the endpoint reports `prometheus_available: false`
and the chart is empty by design.

> **Blank chart troubleshooting:** an empty mail-flow chart most often means
> Prometheus cannot **scrape KumoMTA** — commonly because KumoMTA's metrics
> endpoint is served over TLS with a certificate that has no IP SAN, so the
> scrape fails. Verify the Prometheus target is up before suspecting Iris.

## Related

- [Configuration](configuration.md) — `prometheus_url`
- [Mail logs](mail-logs.md)

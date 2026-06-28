# Queues & service control

These two operational views let you observe and steer the running KumoMTA without
editing config.

## Queues

UI: **Operations → Queues** (`queue:read`, control needs `queue:control`).

KumoMTA organizes outbound mail into **scheduled queues**. The Queues view shows
a summary per queue — state (running / paused / draining), depth, and the age of
the oldest message — and lets you act on a queue:

| Action | Effect |
| ------ | ------ |
| **Suspend** | Pause delivery for the queue (optionally with a reason) |
| **Resume** | Un-pause a suspended queue |
| **Bounce** | Administratively fail the queued messages (emits `AdminBounce` log records) |

Commands are published to the `iris.queue.commands` Redis stream and executed by
the `service-control` worker against `kumod`'s admin API, so the action is
decoupled from the request and retried independently of the UI.

## Service control

UI: **Operations → Service Control** (`service:control`).

Service control covers process-level actions — reloading or restarting KumoMTA
after a [config apply](kumomta-config.md). Requests are serialized through a
service-control request table (so two applies can't race), executed by the
`service-control` worker via the configured reload/restart hooks, and
[audited](audit-log.md).

> Reload vs restart matters: listener/spool/log-hook changes need a **restart**;
> everything else can **reload**. Iris picks the right one and tells you when a
> restart is required but no restart hook is configured. See
> [KumoMTA config generation](kumomta-config.md).

## Related

- [KumoMTA config generation](kumomta-config.md)
- [Worker errors](worker-errors.md)

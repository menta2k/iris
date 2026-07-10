# Iris documentation

Documentation for the Iris KumoMTA admin UI and control plane. For a project
overview and quick start, see the [root README](../README.md).

## Foundations

| Doc | What it covers |
| --- | --- |
| [Architecture](architecture.md) | Control-plane model, components, the Redis event bus, data flow |
| [Configuration](configuration.md) | Config file, environment variables, and UI-managed global settings |
| [Deployment](deployment.md) | Docker Compose, container images, and the Debian/RPM systemd package |
| [KumoMTA config generation](kumomta-config.md) | How UI config is rendered to Lua and applied (reload vs restart) |
| [API & Swagger](swagger.md) | The generated OpenAPI document and how to browse it |

## Security

| Doc | What it covers |
| --- | --- |
| [Authentication](authentication.md) | Password login, TOTP MFA, session tokens, bootstrapping the first admin |
| [Authorization](authorization.md) | Roles, permissions (RBAC), and user management |
| [Audit log](audit-log.md) | What is audited and how to read it |

## Outbound configuration

| Doc | What it covers |
| --- | --- |
| [Listeners](listeners.md) | Inbound (MX) and submission listeners; the relay allowlist model |
| [VMTAs](vmtas.md) | Egress sources: IP address and EHLO identity |
| [VMTA groups](vmta-groups.md) | Weighted egress pools |
| [Routing rules](routing-rules.md) | Classifying mail into a mailclass and pinning egress |

## Deliverability & domain safety

| Doc | What it covers |
| --- | --- |
| [DKIM](dkim.md) | DKIM key management and signing |
| [Suppressions](suppressions.md) | The Redis-backed suppression list and TTLs |
| [TLS Policy](tls-policies.md) | Per-domain outbound TLS: require, relax, or disable (for broken-cert receivers) |
| [Bounce handling](bounce-handling.md) | DSN capture, VERP, classification, auto-suppression |
| [Feedback loops](feedback-loops.md) | ARF complaint ingestion and provenance verification |
| [DMARC reports](dmarc.md) | Aggregate-report capture and parsing |
| [Inbox monitoring](inbox-monitoring.md) | Seed-mailbox probes: inbox-vs-spam placement and header-based spam-risk analysis |

## Inbound

| Doc | What it covers |
| --- | --- |
| [Inbound routing](inbound-routing.md) | Maildir / forward / webhook routes for hosted domains |
| [Rspamd spam filtering](rspamd.md) | Inbound scanning, modes, and the results view |

## Operations

| Doc | What it covers |
| --- | --- |
| [Mail logs](mail-logs.md) | Log streaming from KumoMTA and the mail record model |
| [Retention & cleanup](retention.md) | Per-table TimescaleDB chunk compression/dropping and disk reclaim |
| [Queues](queues.md) | Inspecting and controlling KumoMTA scheduled queues |
| [Worker errors](worker-errors.md) | The generic background-worker error log |
| [Dashboard & metrics](dashboard.md) | The summary dashboard and Prometheus time-series |
| [ACME / TLS certificates](acme.md) | Let's Encrypt issuance and auto-renewal |
| [Domain bounce readiness](domain-check.md) | MX/SPF/DKIM checks for your sending domains |
| [Diagnostics](diagnostics.md) | Sender diagnose and RBL/DNSBL checks |

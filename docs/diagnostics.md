# Diagnostics

Two read-only tools help you debug deliverability for a sender or IP using live
DNS. Both are under **Tools** (`service:control`).

## Sender diagnose

UI: **Tools → Diagnose.**

Given a sender (address or domain), Iris runs live DNS checks and summarizes the
authentication posture a receiver would see — MX, SPF, DKIM, and DMARC alignment
signals — so you can answer "why is this sender failing auth?" without leaving
the UI. It reuses the same DNS machinery as
[Domain Bounce Readiness](domain-check.md) but framed around a sending identity.

## RBL / DNSBL check

UI: **Tools → RBL Check.**

Checks an IP address against well-known DNS blocklists (RBL/DNSBL) via live DNS
lookups and reports which lists, if any, have the IP listed. Use it to check
whether one of your [egress IPs](vmtas.md) has landed on a blocklist that would
hurt delivery.

## Related

- [Domain bounce readiness](domain-check.md)
- [VMTAs](vmtas.md)

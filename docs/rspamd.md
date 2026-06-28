# Rspamd spam filtering

Iris can scan **inbound** mail through an [Rspamd](https://rspamd.com) instance,
add spam headers, and (optionally) reject spam at SMTP time. Scanning runs inside
the KumoMTA reception hook against Rspamd's `/checkv2` endpoint.

UI: results at **Inbound Automation → Rspamd Results** (`rspamd:read`). Mode and
URL are set in **Global Settings** (or process config `rspamd.*`).

## Modes

The deployment-wide `rspamd_mode` global setting:

| Mode | Behavior |
| ---- | -------- |
| `off` (or empty) | No scanning |
| `tag` | Scan, add `X-Spam-Score` / `X-Rspamd-Action` headers, **never reject** |
| `enforce` | Honor Rspamd's verdict — reject (`reject`) or greylist (`soft reject`) |

Scanning **fails open**: if Rspamd is unreachable or errors, the message is not
blocked.

## Scope

Scanning is scoped to **inbound-to-hosted** mail only — never outbound relay or
system mail. The hosted-domain set is the explicit hosted domains, or (when not
set) is derived from your [DKIM](dkim.md) domains. **Inbound-route domains are
always in scope**, so maildir/forward/webhook mail is scanned.

## Per-route override

Individual [inbound routes](inbound-routing.md) can override the global mode with
their own `spam_scan` (`default` / `off` / `tag` / `enforce`). The scan machinery
is rendered whenever the global mode is on **or** any route opts in — so a single
route can enforce scanning even when the deployment default is `off` (provided an
Rspamd URL is configured).

## Results

Each verdict is XADDed onto the `iris.rspamd.results` Redis stream (with the
KumoMTA message id), consumed by the `rspamd-ingest` worker, and stored. The
Rspamd Results page shows the **recipient** and **message id** (resolved from the
mail log), score, action, the symbols that fired, and the reason.

## Related

- [Inbound routing](inbound-routing.md)
- [Mail logs](mail-logs.md)

# Inbox-placement monitoring

Inbox monitoring measures **where your mail actually lands** at real providers.
Iris sends a uniquely-tagged *probe* message to a mailbox you control (a "seed"
inbox), tracks it through KumoMTA, then logs into that mailbox over IMAP/POP3 to
find it, record which folder it landed in, and analyze its headers for spam
signals. It answers the question deliverability dashboards can't: *inbox or
spam?*

UI: **Monitoring → Inbox Monitoring**. Reading requires `monitoring:read`;
adding mailboxes and sending probes requires `monitoring:write` (granted to the
operator role; viewers get read-only).

## How it works

A probe runs through three phases:

1. **Send** — iris injects a probe message into KumoMTA addressed to the seed
   mailbox. The message carries a unique id (`probeUid`) in three places: an
   `X-Iris-Probe-Id` header, the subject (`[iris-probe] <uid>`), and a
   plus-tagged `From` address (`probe+<uid>@yourdomain`). A background reconciler
   correlates the probe against the [mail log](mail-logs.md) by that tagged
   `From` + recipient and advances its **send status** (queued → sent / deferred
   / bounced).
2. **Fetch** — after a per-account **fetch delay** (default 10 min, to let the
   provider deliver and filter), the fetch worker logs into the mailbox, searches
   the configured folders for the probe id, and records the **mailbox status**
   (found / not_found / timeout), the **placement** (which folder it was in), the
   **latency**, and the message's raw headers.
3. **Analyze** — the captured headers are scored for **spam risk**: SPF/DKIM/
   DMARC results plus spam-score headers are parsed deterministically, and — if
   an OpenAI key is configured — an LLM produces a `clean` / `suspicious` /
   `spam` verdict with a short rationale. See [Spam-risk analysis](#spam-risk-analysis).

Placement (where the message physically landed) and spam-risk (what the headers
suggest) are shown separately. For IMAP accounts the folder is ground truth;
for POP3 (which only exposes the inbox) the header verdict is the primary
spam signal.

## Setup

### 1. Configure the encryption key (required)

Mailbox passwords must be stored **reversibly** (iris presents them to the
IMAP/POP3 server), so they are encrypted with AES-256-GCM keyed by an operator
passphrase. Set it before adding any mailbox with a password:

```bash
# any non-empty string; keep it stable and secret (rotating it invalidates
# every stored mailbox password)
export IRIS_MONITORING_KEY="$(openssl rand -hex 32)"
```

If `IRIS_MONITORING_KEY` is unset, iris still runs but **refuses to store
mailbox passwords** (you'll see a startup warning); accounts can be created but
probes to them can't be fetched.

### 2. Set a default probe sender (recommended)

Every probe needs a `From` domain iris can send from and ideally
[DKIM-sign](dkim.md) (so the probe itself authenticates). Set a fallback used by
any account that doesn't define its own `from_address`:

```bash
export IRIS_MONITORING_FROM="probe@monitor.example.com"
```

The domain must be one of your sending domains — resolvable, SPF-authorized for
your egress, and DKIM-signed — otherwise probes will look unauthenticated and
skew the spam verdict.

### 3. Add a mailbox

**Monitoring → Inbox Monitoring → Add Mailbox**. Choosing a **provider**
prefills the connection details; pick **Custom** to edit them.

| Field | Notes |
| --- | --- |
| Provider | `gmail`, `outlook`, `yahoo`, or `custom` (presets fill host/port/protocol) |
| Mailbox address | The seed address probes are sent to |
| Protocol | `imap` (folder-aware, recommended) or `pop3` (inbox only) |
| Host / Port / TLS | Prefilled per provider; e.g. `imap.gmail.com:993` TLS |
| Username | Defaults to the mailbox address |
| Password | **App password** — see below. Write-only; never shown again |
| Folders to search | IMAP only, e.g. `INBOX`, `[Gmail]/Spam`. Default `INBOX` |
| From address | Per-account probe sender; blank uses `IRIS_MONITORING_FROM` |
| Schedule | Optionally send a probe automatically every interval (e.g. `6h`) |
| Fetch delay | How long to wait after send before checking (default `10m`) |

**Use an app password, not the account password**, for providers with 2FA:

- **Gmail/Yahoo**: create an *app password* (requires 2-step verification) and,
  for Gmail, ensure **IMAP is enabled** in settings. To detect spam placement,
  add `[Gmail]/Spam` to the folder list.
- **Microsoft 365 / Outlook**: use an app password or a mailbox with basic
  IMAP auth enabled. OAuth2-only tenants are not yet supported (see
  [Limitations](#limitations)).

### 4. Send a probe

Click **Send test** on a mailbox row (or open **Probes** for its history and
send from there). Send status appears within seconds; toggle **Live** on the
Probes page to poll as the fetch and analysis complete. To run probes
automatically, enable the mailbox's **schedule**.

### 5. Enable spam-risk analysis (optional)

Deterministic SPF/DKIM/DMARC parsing always runs. To add the LLM verdict, set an
OpenAI key (shared with [subject classification](configuration.md)):

```bash
export IRIS_OPENAI_API_KEY="sk-..."
export IRIS_OPENAI_MODEL="gpt-4o-mini"          # optional, this is the default
export IRIS_OPENAI_API_BASE="https://api.openai.com/v1"  # optional; override for Azure/gateway
```

Without a key, the **Spam risk** column still shows a heuristic verdict from the
parsed auth results and spam headers.

## Spam-risk analysis

The **Spam risk** column on the Probes page combines two layers:

- **Deterministic (always):** `Authentication-Results` is parsed for SPF, DKIM,
  and DMARC; SpamAssassin/rspamd headers (`X-Spam-Flag`, `X-Spam-Score`,
  `X-Spam-Status`) are read for a score/flag. A heuristic verdict follows:
  both SPF+DKIM failing or a spam flag ⇒ `spam`; DMARC fail or a single auth
  failure ⇒ `suspicious`; clean auth ⇒ `clean`.
- **LLM (when `IRIS_OPENAI_API_KEY` is set):** the headers are sent to an
  OpenAI-compatible endpoint, which returns a `clean`/`suspicious`/`spam`
  verdict, a confidence, a one-line summary, and the notable factors. Rows
  scored by the model are tagged **AI**.

The verdict is **advisory** — it never overrides the folder-based placement. On
any LLM error the deterministic verdict is kept, so an analysis always exists.

## Background workers & tuning

Three workers drive the pipeline; all are enabled automatically and tunable by
environment variable:

| Worker | Job | Interval env (default) |
| --- | --- | --- |
| `monitoring-reconciler` | Correlate probes to the mail log; set send status | `IRIS_MONITORING_RECONCILE_INTERVAL` (`30s`) |
| `monitoring-scheduler` | Send probes for accounts whose schedule is due | `IRIS_MONITORING_SCHEDULE_INTERVAL` (`1m`) |
| `monitoring-fetch` | Log into mailboxes, find probes, analyze headers | `IRIS_MONITORING_FETCH_INTERVAL` (`1m`) |

Additional knobs:

| Variable | Default | Purpose |
| --- | --- | --- |
| `IRIS_MONITORING_KEY` | *(unset)* | AES-GCM passphrase for mailbox passwords (required to store credentials) |
| `IRIS_MONITORING_FROM` | *(unset)* | Fallback probe sender when an account has no `from_address` |
| `IRIS_MONITORING_RECONCILE_LOOKBACK` | `1h` | How far back the reconciler considers queued probes |
| `IRIS_MONITORING_FETCH_TIMEOUT` | `30s` | Per-connection IMAP/POP3 timeout |
| `IRIS_MONITORING_FETCH_GIVEUP` | `2h` | How long to retry the fetch before marking a probe not_found / timeout |
| `IRIS_OPENAI_API_KEY` | *(unset)* | Enables the LLM spam verdict (shared with subject classification) |
| `IRIS_OPENAI_MODEL` | `gpt-4o-mini` | Model for the LLM analysis |
| `IRIS_OPENAI_API_BASE` | `https://api.openai.com/v1` | Override for Azure OpenAI or a local gateway |

## Data model

Two TimescaleDB tables (migration `0055`):

- **`monitoring_accounts`** — the seed mailboxes: connection details, the
  AES-GCM-encrypted `password_enc`, folders, schedule, and fetch delay.
- **`monitoring_probes`** — one row per probe: `probe_uid`, correlated KumoMTA
  `message_id`, `send_status`, `mailbox_status`, `placement`, `latency_ms`, the
  captured `raw_headers`, and the phase-3 `analysis` JSON.

## Security

- Mailbox passwords are **write-only** in the API — create/update accept a
  password but no endpoint ever returns it. Reads expose only a `hasPassword`
  boolean.
- Passwords are stored **AES-256-GCM encrypted** at rest, keyed by
  `IRIS_MONITORING_KEY`. Losing or rotating the key makes existing stored
  passwords undecryptable (re-enter them via **Password** on each account).
- Mailbox actions are recorded in the [audit log](audit-log.md)
  (`monitoring_account.*`, `monitoring_probe.send`).

## Gotchas

- **No `IRIS_MONITORING_KEY`** ⇒ passwords can't be stored and fetches are
  skipped. Check startup logs for the warning.
- **`from_address` must be a real sending domain.** A domain iris can't
  SPF/DKIM-authenticate makes every probe look unauthenticated and the verdict
  lean toward spam.
- **Gmail spam detection needs the Spam folder listed.** Add `[Gmail]/Spam` to
  the account's folders, or a probe filtered to spam reads as `not_found`.
- **POP3 can't see the spam folder.** Placement for POP3 accounts is always
  inbox (the only retrievable folder); rely on the header **spam-risk** verdict
  instead.
- **Fetch delay vs. give-up.** A probe stays `pending` and retries until
  `IRIS_MONITORING_FETCH_GIVEUP`; a mailbox that's slow or greylisted needs a
  give-up window longer than its worst-case delivery time.

## Limitations

- **App-password / basic auth only.** OAuth2 (Gmail, Microsoft 365 modern auth)
  is not yet supported.
- The **Live** view on the Probes page polls; it is not a push stream.

## Related

- [Mail logs](mail-logs.md) — the record store probe send status is correlated against
- [DKIM](dkim.md) — sign your probe `from_address` so probes authenticate
- [DMARC reports](dmarc.md) — aggregate authentication reporting for your domains
- [Configuration](configuration.md) — full environment-variable reference
- [Authorization](authorization.md) — the `monitoring:read` / `monitoring:write` permissions

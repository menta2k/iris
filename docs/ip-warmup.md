# IP Warmup

IP warmup gradually increases the volume iris sends from a new egress IP (VMTA),
per receiving-domain family (MBP), so mailbox providers build trust in the IP's
reputation instead of seeing a cold IP suddenly blast high volume (which gets
throttled, deferred, or blocked).

This document describes the **currently implemented** behavior and walks through
practical examples. Planned enhancements are called out in
[Roadmap](#roadmap).

---

## How it works (in one paragraph)

You attach a **warmup schedule** to a VMTA: a **start date** and a **curve**
(a template of daily caps). Each day the schedule is on a **ramp day**
(`today − start_date + 1`). For that day, iris looks up a **messages-per-day cap
per MBP bucket** (Gmail, Microsoft, Yahoo, default) and renders it into the
KumoMTA policy as a `max_message_rate` throttle on that IP's egress path for that
provider. When the ramp passes the end of the curve, the schedule **completes**
and the cap is **removed entirely** — the IP then sends at your normal limits.

The MBP buckets exist because **reputation is per-provider**: Gmail tracking your
IP is independent of Microsoft tracking it, so each is warmed on its own cap.

---

## Concepts

| Term | Meaning |
|------|---------|
| **VMTA / egress source** | The sending IP being warmed. |
| **MBP bucket** | Receiving-domain family: `gmail`, `microsoft`, `yahoo`, `default`. |
| **Curve** | A template of stages, each a day-range with a per-bucket daily cap. |
| **Ramp day** | 1-based day index: `today − start_date + 1`. Day 1 = the start date. |
| **Stage** | A contiguous day-range (e.g. days 9–11) with one cap per bucket. |
| **Status** | `scheduled` → `active` → `completed`, plus `paused`. |
| **Held day** | The frozen ramp day while paused (so the cap holds exactly). |

### MBP bucket → receiving domains
| Bucket | Example recipient domains |
|--------|---------------------------|
| `gmail` | gmail.com, googlemail.com |
| `microsoft` | outlook.com, hotmail.com, live.com, msn.com, hotmail.co.uk, outlook.co.uk |
| `yahoo` | yahoo.com, yahoo.co.uk, ymail.com, rocketmail.com, aol.com |
| `default` | every other recipient domain |

A recipient domain that isn't in the first three buckets uses the `default` cap.
If a stage sets no cap for a bucket, it falls back to that stage's `default`.

---

## The built-in curves

### `standard` (~21 days — the sensible default)
| Ramp days | gmail | microsoft | yahoo | default |
|-----------|------:|----------:|------:|--------:|
| 1–2 | 50 | 50 | 50 | 200 |
| 3–4 | 100 | 100 | 100 | 500 |
| 5–6 | 500 | 300 | 300 | 1,000 |
| 7–8 | 1,000 | 500 | 500 | 5,000 |
| 9–11 | 5,000 | 2,000 | 2,000 | 20,000 |
| 12–14 | 20,000 | 10,000 | 10,000 | 50,000 |
| 15–17 | 75,000 | 40,000 | 40,000 | 150,000 |
| 18–21 | 200,000 | 100,000 | 100,000 | 500,000 |
| 22+ | — completed: cap removed — |

### `conservative` (~30 days — cold IPs / strict audiences)
| Ramp days | gmail | microsoft | yahoo | default |
|-----------|------:|----------:|------:|--------:|
| 1–3 | 20 | 20 | 20 | 100 |
| 4–6 | 50 | 50 | 50 | 200 |
| 7–9 | 100 | 100 | 100 | 500 |
| 10–13 | 500 | 300 | 300 | 2,000 |
| 14–17 | 2,000 | 1,000 | 1,000 | 10,000 |
| 18–22 | 10,000 | 5,000 | 5,000 | 30,000 |
| 23–27 | 40,000 | 20,000 | 20,000 | 100,000 |
| 28–30 | 100,000 | 50,000 | 50,000 | 300,000 |

### `aggressive` (~12 days — only for domains with existing reputation)
| Ramp days | gmail | microsoft | yahoo | default |
|-----------|------:|----------:|------:|--------:|
| 1 | 200 | 100 | 100 | 500 |
| 2–3 | 1,000 | 500 | 500 | 2,000 |
| 4–5 | 5,000 | 2,000 | 2,000 | 20,000 |
| 6–8 | 25,000 | 10,000 | 10,000 | 75,000 |
| 9–12 | 150,000 | 75,000 | 75,000 | 400,000 |

> Microsoft and Yahoo ramp more conservatively than Gmail because their
> reputation gates are stricter; the `default` long tail is the most generous.

---

## Example 1 — Warm a new IP with the `standard` curve

**Setup.** VMTA `vmta-04` sends `203.0.113.10`. On **2026-07-01** you create a
warmup with curve `standard`, start date `2026-07-01`.

**What happens day by day** (the cap iris enforces per provider):

| Date | Ramp day | gmail | microsoft | yahoo | everything else |
|------|---------:|------:|----------:|------:|----------------:|
| Jul 1 | 1 | 50/day | 50/day | 50/day | 200/day |
| Jul 2 | 2 | 50/day | 50/day | 50/day | 200/day |
| Jul 3 | 3 | 100/day | 100/day | 100/day | 500/day |
| … | … | … | … | … | … |
| Jul 10 | 10 | 5,000/day | 2,000/day | 2,000/day | 20,000/day |
| Jul 21 | 21 | 200,000/day | 100,000/day | 100,000/day | 500,000/day |
| Jul 22 | 22 | **no cap** | **no cap** | **no cap** | **no cap** |

So on **Jul 1**, if you try to send 1,000 messages to `gmail.com` from this IP,
KumoMTA paces delivery to **50 per rolling day** and the rest waits in queue. On
**Jul 22** the warmup is complete and the IP sends at your normal limits.

**What iris renders into the KumoMTA policy** on Jul 1 (ramp day 1):

```lua
WARMUP_RATE["vmta-04"] = { ["gmail"] = "50/day", ["microsoft"] = "50/day",
                           ["yahoo"] = "50/day", ["default"] = "200/day", }

kumo.on('get_egress_path_config', function(domain, egress_source, site_name)
  local params = { ... }
  ...
  local wr = WARMUP_RATE[egress_source]
  if wr then
    local rate = wr[warmup_bucket(domain)] or wr['default']
    if rate then params.max_message_rate = rate end
  end
  ...
end)
```

`warmup_bucket("gmail.com")` returns `"gmail"`, so mail from `vmta-04` to
`gmail.com` gets `max_message_rate = "50/day"`. Mail to, say, `proton.me`
(not a tracked bucket) gets the `default` `"200/day"`.

> **`N/day` is a rolling-window rate, not a midnight-reset quota.** KumoMTA paces
> to roughly N per rolling 24h and smooths bursts. It is the warmup *ceiling*; it
> does not force you to send N — if you queue less, you send less.

---

## Example 2 — Per-provider caps differ within a day

Buckets are independent. On **Jul 10** (ramp day 10, `standard`) the same IP
simultaneously enforces:

- `gmail.com`, `googlemail.com` → **5,000/day**
- `outlook.com`, `hotmail.com`, `live.com` → **2,000/day**
- `yahoo.com`, `aol.com` → **2,000/day**
- `acme.org`, `proton.me`, anything else → **20,000/day**

If your campaign is 50% Gmail and 50% long-tail, Gmail is the binding constraint
at 5,000/day while the long tail flows up to 20,000/day — exactly the point of
per-provider warmup.

---

## Example 3 — Pause and resume

Pausing **freezes the ramp at the current cap** (e.g. you saw a deferral spike
and want to hold before increasing). Resume continues from where you left off.

**Timeline.** `standard` curve, started Jul 1.

1. **Jul 8** (ramp day 8): caps are gmail `1,000/day`. You **pause** with reason
   "watching deferrals". iris records `held_day = 8`, status `paused`.
2. **Jul 8 → Jul 12**: time passes, but because the schedule is paused the cap
   **stays at day-8 levels** (gmail `1,000/day`) — it does **not** advance to the
   day 9–11 stage. Mail keeps flowing at the held cap.
3. **Jul 12**: you **resume**. iris shifts the start date forward by the 4 paused
   days (new start `2026-07-05`) so that *today* maps back to **ramp day 8**, then
   sets status `active`. The next day (Jul 13) advances to ramp day 9 → gmail
   `5,000/day`, and the ramp continues normally from there.

Net effect: pausing for 4 days lengthens the warmup by 4 days; you never skip a
stage or jump the cap.

---

## Lifecycle

```
            start date reached                 ramp day > curve end
  scheduled ─────────────────► active ───────────────────────────► completed
                                  │  ▲                                 (cap removed)
                          pause   │  │  resume (shifts start_date
                                  ▼  │   so today = held_day)
                                paused
```

- **scheduled** — start date is in the future; no cap applied yet.
- **active** — ramping; the day's per-bucket cap is enforced.
- **paused** — frozen at `held_day`; the cap holds exactly until resumed.
- **completed** — past the curve; no warmup cap (normal limits apply).

One non-completed schedule per VMTA. Editing a schedule re-derives status from
the start date and clears any pause.

---

## How the schedule advances (the worker)

A background **warmup worker** runs on a cadence (`IRIS_WARMUP_INTERVAL`,
default **1h**). Each tick it:

1. Advances lifecycle: `scheduled → active` when the start date arrives;
   `active → completed` once the ramp passes the curve's last day.
2. Computes today's resolved caps for every active/paused schedule and compares
   them to the last-applied set (a **fingerprint**). **Only if the caps changed**
   — a new stage began, a schedule started, completed, or was paused/resumed —
   does it re-apply the KumoMTA policy so the new `max_message_rate` goes live.

Because of the fingerprint gate, a multi-day stage (e.g. days 9–11) triggers
exactly **one** re-apply when it begins, not one per hour or per day. Crossing
into a new stage at a day boundary applies within one interval (≤1h by default).

> A reload (not a restart) is used, so picking up the new day's cap is low-impact.

---

## Setting it up (UI)

**Outbound → IP Warmup → New warmup:**
1. Pick the **VMTA** to warm.
2. Pick a **curve** (`standard` / `conservative` / `aggressive`).
3. Pick a **start date** (day 1 of the ramp; UTC).

The IP Warmup page then shows, per schedule: the **status**, **progress**
(`day X / N`), and **today's caps** per provider. Use **Pause/Resume** as needed.

You can confirm the active cap is correct by watching **Operations → Mail Logs**:
during warmup, deliveries to a capped provider pace out rather than all sending
at once.

---

## Operational guidance

- **Send consistent volume.** Warmup works best with steady daily sending into
  the cap, not sporadic bursts. An empty day wastes a ramp day.
- **Engaged recipients first.** Early stages (tens–hundreds/day) should target
  your most engaged recipients to maximize opens/replies and positive signals.
- **Watch deferrals/bounces.** If a provider starts deferring (4xx) or bouncing,
  **pause** rather than push through the wall; resume once it settles.
- **One IP per schedule.** Warm each new IP independently; reputation is per-IP.
- **Pick the curve to your reputation.** New domain/cold IP → `conservative`;
  established domain adding capacity → `standard`/`aggressive`.

---

## Reference (where this lives in the code)

| Concern | Location |
|---------|----------|
| Curve math (`CapFor`, day index, completion) | `backend/internal/biz/warmup.go` |
| Built-in curves | `backend/internal/biz/warmup_curves.go` |
| Lifecycle (create/pause/resume/tick) | `backend/internal/biz/warmup_usecase.go` |
| Rendered policy (`WARMUP_RATE`, `warmup_bucket`) | `backend/internal/biz/kumo_config.go` |
| Worker (cadence + fingerprint re-apply) | `backend/internal/worker/warmup_worker.go` |
| API | `backend/internal/service/warmup_service.go` |
| UI | `frontend/src/pages/outbound/WarmupPage.vue` |

---

## Shaping-helper enforcement (available, opt-in)

The per-day cap can now be enforced through KumoMTA's **shaping helper** instead
of the custom `get_egress_path_config`: iris renders the warmup cap as a per-IP
override in `iris-warmup.toml`, layered over the **Delivery Blueprints** base
(`iris-base.toml`), and the policy resolves limits via `kumo.shaping.load`. The
operator sees the **same enforcement** (the warming IP is still capped per
provider per day); the mechanism is just native — it uses KumoMTA's provider
grouping and is ready to gain TSA protection.

- **Enable:** set `IRIS_SHAPING_ENABLE=true`. The apply adapter writes
  `iris-base.toml` + `iris-warmup.toml` next to the policy and the policy loads
  them. Unset (default) keeps the legacy `MBP_BUCKET` path — same behavior.
- **Verified** against a live `kumod` (`--validate`): the full policy boots and
  resolution is correct — a warming source resolves to its warmup override
  (e.g. `50/day`) while other sources get the blueprint base (e.g. `150/day`),
  both with the blueprint's connection settings.
- Layer details: `docs/delivery-blueprints-and-warmup.md`.

## Roadmap

Still **planned**:

- **Expanding ISP targeting** — start by warming only a subset of providers
  (e.g. Gmail + Yahoo) and add more (Microsoft, regional) at later stages, rather
  than ramping all buckets from day 1.
- **Hourly peak / adaptive throttling (TSA)** — a per-hour smoothing limit and
  reactive back-off on deferrals via KumoMTA Traffic Shaping Automation, layered
  under the warmup ceiling. Builds directly on the shaping-helper enforcement
  above.
- **Retire `MBP_BUCKET`** — once the shaping path has soaked on, make it the
  default and remove the legacy custom egress-path code (P2).

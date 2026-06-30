# Enabling & configuring IP warmup — step by step

A practical guide to warming a new sending IP in iris. For how the ramp behaves
(curves, day-by-day caps, pause math), see [`ip-warmup.md`](./ip-warmup.md); for
the layered architecture, see
[`delivery-blueprints-and-warmup.md`](./delivery-blueprints-and-warmup.md).

> Enforcement runs through KumoMTA's shaping helper by default — there is **no
> feature flag to turn on**. You configure blueprints + a warmup schedule, then
> apply the config.

---

## 0. Prerequisites (do these first — warmup paces volume, it does not fix auth)

Warmup only controls *how fast* you ramp; the IP still needs correct sending
identity or providers will reject it regardless of pace:

- [ ] A **VMTA** exists for the new IP — Outbound → VMTAs → New VMTA, with the
      egress **IP address** and **EHLO name**.
- [ ] **rDNS / PTR** for the IP resolves to your EHLO hostname.
- [ ] **SPF** for your sending domains authorizes the IP.
- [ ] **DKIM** signing is configured for the From-domain (Domain Safety → DKIM
      Domains, status `ready`, key published). Use Tools → Diagnose to confirm.
- [ ] The backend can write the KumoMTA policy (`kumomta.config_path` set and
      `stub: false`), and you have the **service-control** permission.

---

## 1. Seed the Delivery Blueprints (base limits / fallback)

Blueprints are the per-provider starting limits a new IP falls back to. They are
the floor the warmup overrides sit on top of.

1. Go to **Outbound → Delivery Blueprints**.
2. Click **Seed Defaults** to import the major providers (Gmail, Microsoft,
   Yahoo) with conservative starting limits.
3. (Optional) **Add Rule** / **Edit** to tune `Conn Rate`, `Deliveries/Conn`,
   `Conn Limit`, and `Daily Cap` per MX pattern, or **Disable** ones you don't
   send to.

---

## 2. Create a warmup schedule for the IP

1. Go to **Outbound → IP Warmup → New warmup**.
2. Pick the **VMTA** to warm.
3. Pick a **Curve**:
   - `standard` (~21 days) — the sensible default.
   - `conservative` (~30 days) — cold IP or strict audience.
   - `aggressive` (~12 days) — only with existing domain reputation.
   - `custom` — define your own stages (day range + per-MBP daily caps); add or
     remove rows. Stages must be 1-based and contiguous.
4. Pick a **Start date** (day 1 of the ramp, UTC). Today is fine.
5. **Create.** If the start date is today/past, the schedule is immediately
   `active`; a future date starts as `scheduled`.

---

## 3. Apply the KumoMTA config (push it live)

Blueprints and the warmup schedule are stored config — they take effect on the
next apply, which writes the policy + the `iris-base.toml` / `iris-warmup.toml`
shaping files next to it and reloads kumod.

1. Go to **Operations → KumoMTA Config**.
2. Click **Generate** and review (it should report **valid**).
3. Click **Apply** and confirm.

After this, the warmup worker keeps the config current automatically: it
re-applies **only when the resolved caps change** (a new stage begins, a
schedule starts/completes, or you pause/resume), so you don't apply by hand each
day.

---

## 4. Verify it's working

- **Outbound → IP Warmup** shows each schedule's **status**, **progress**
  (`day X / N`), and **today's caps** per provider.
- **Operations → Mail Logs** — during warmup, deliveries to a capped provider
  pace out instead of all sending at once.
- **Tools → Diagnose** (enter a From address) — confirms SPF/DKIM/DMARC and
  bounce-domain alignment for the sending domain.

---

## 5. Operate the ramp

- **Send consistent volume** into the cap every day — an empty day wastes a ramp
  day. Start with your most engaged recipients.
- If a provider starts **deferring (4xx) or bouncing**, click **Pause** (holds
  the current cap exactly) rather than pushing through; **Resume** once it
  settles. Pausing N days lengthens the warmup by N days; it never skips a stage.
- When the ramp passes the curve's last day it **completes** and the cap is
  **removed** — the IP then sends at your normal (blueprint) limits.

---

## 6. (Optional) Enable adaptive throttling (TSA)

TSA adds reactive, hourly back-off *under* the warmup ceiling (it only tightens
on bad signals, never raises). It needs a `tsa-daemon` running.

1. Start the daemon (local/dev):
   ```sh
   docker compose -f deploy/compose/docker-compose.yml --profile tsa up -d tsa-daemon
   ```
2. Point the backend at it and restart it:
   ```sh
   export IRIS_TSA_URL="http://localhost:8008"   # or http://tsa-daemon:8008 in-network
   ```
3. **Apply** the KumoMTA config again (step 3) so the policy publishes/subscribes
   to the daemon.

See `deploy/compose/README.md` for details.

---

## Quick reference

| What | Where |
|------|-------|
| Base per-provider limits | Outbound → Delivery Blueprints (Seed Defaults / Add Rule) |
| Warmup schedule | Outbound → IP Warmup → New warmup |
| Push config live | Operations → KumoMTA Config → Generate → Apply |
| Progress / today's caps | Outbound → IP Warmup |
| Auth check | Tools → Diagnose |
| Worker cadence | `IRIS_WARMUP_INTERVAL` (default 1h) |
| Adaptive throttling | `IRIS_TSA_URL` + `tsa-daemon` |

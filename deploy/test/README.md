# iris e2e test harness

End-to-end exercise of the full pipeline:

```
loadgen  ──SMTP:2525──▶  kumomta  ──┬─▶ mock-mta-accept   (200 ⇒ Delivery)
                                    ├─▶ mock-mta-bounce   (550 ⇒ Bounce)
                                    ├─▶ mock-mta-defer    (450 ⇒ TransientFailure)
                                    ├─▶ mock-mta-slow     (sleep then 200)
                                    └─▶ mock-mta-fbl      (200 + synthetic Feedback into Redis)

kumomta  ──XADD──▶  redis  ◀──XREADGROUP──  admin-service  ──INSERT──▶  timescaledb
```

## Run

```sh
cd deploy
docker compose \
    -f docker-compose.yaml \
    -f docker-compose.test.yaml \
    --profile test \
    up --build --abort-on-container-exit loadgen
```

The loadgen seeds VMTAs, groups, classes, and routing rules (idempotent),
calls `/v1/policy/apply`, then drives traffic according to
[scenarios/mixed.yaml](scenarios/mixed.yaml). After traffic drains it
queries the admin API and asserts on observed `log_event` counts,
`feedback_reports` totals, and `suppression_entries` reasons. Exit code is
the assertion verdict; `--abort-on-container-exit` propagates that to
docker compose so CI can fail the run cleanly.

## Files

| Path                                         | Purpose                                         |
|----------------------------------------------|-------------------------------------------------|
| `deploy/test/mocks/Dockerfile`               | aiosmtpd-based mock MTA, behavior via env.      |
| `deploy/test/mocks/mockmta.py`               | The 5 behaviors live here.                      |
| `deploy/test/scenarios/mixed.yaml`           | Default scenario.                               |
| `deploy/docker-compose.test.yaml`            | Layered services (mocks + loadgen).             |
| `backend/scripts/loadgen/`                   | Go harness (scenario loader + runner + asserts).|

## Authoring new scenarios

Drop a `.yaml` next to `mixed.yaml` and pass its container path via
`-scenario=/scenarios/<name>.yaml`. Schema is in
[`backend/scripts/loadgen/scenario.go`](../../backend/scripts/loadgen/scenario.go).

The renderer-side knob that pins a recipient domain at a mock is
`IRIS_TEST_DOMAIN_ROUTES` — a JSON object mapping `domain → host:port`.
The compose file populates it from the five mocks; new scenarios that need
an extra mock domain should add it there.

## Mock-MTA behaviors

| `MOCK_BEHAVIOR` | Response          | Drives                                  |
|-----------------|-------------------|-----------------------------------------|
| `accept`        | `250`             | `Reception` + `Delivery`                |
| `bounce`        | `550`             | `Reception` + `Bounce`                  |
| `defer`         | `450`             | `Reception` + `TransientFailure`        |
| `slow`          | `250` after sleep | timeout / connection-cap testing        |
| `fbl`           | `250` + XADD      | `Reception` + `Delivery` + `Feedback` (synthetic, bypasses kumomta-side ARF parsing — exercises the consumer + DB + auto-suppress path) |

Per-recipient overrides via `MOCK_RULES_JSON` let one container mix
outcomes for tests that exercise that specifically:

```yaml
mock-mta-accept:
  environment:
    MOCK_BEHAVIOR: accept
    MOCK_RULES_JSON: '{"bounce@*": "550 user unknown"}'
```

## Manual UI walkthrough

While the loadgen runs, the SPA at <http://localhost:5173> should populate:

- **Logs** (`/observability/logs`) — every event_type with non-zero counts
- **Feedback Reports** (`/inbound/feedback`) — ~25 rows from the FBL scenario
- **Suppressions** (`/policy/suppressions`) — auto-added rows with `reason=complaint`
- **Audit** (`/observability/audit`) — every API mutation done by the loadgen

"""Configurable SMTP mock destination for iris end-to-end tests.

One image, one of five behaviors selected via MOCK_BEHAVIOR:

  accept  — return 250 OK and discard. Drives Reception + Delivery in kumomta.
  bounce  — return 550 with a permanent-failure DSN. Drives Bounce.
  defer   — return 450 with a transient-failure DSN. Drives TransientFailure.
  slow    — sleep MOCK_DELAY_SEC before returning 250. Tests timeouts/conn caps.
  fbl     — return 250 (so kumomta records Delivery) AND inject a synthetic
            Feedback log_record directly into the Redis stream (bypasses
            kumomta-side ARF parsing, which we don't configure in this round).

Behavior overrides per recipient: MOCK_RULES_JSON='{"bounce@*": "550 user
unknown"}' lets one container mix outcomes within a single test scenario.

The container binds on 0.0.0.0:25 and logs every transaction to stdout so
docker compose logs is the audit trail for the test.
"""

import asyncio
import fnmatch
import json
import logging
import os
import sys
import time

from aiosmtpd.controller import Controller


def _env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


BEHAVIOR = _env("MOCK_BEHAVIOR", "accept")
DELAY_SEC = int(_env("MOCK_DELAY_SEC", "10") or "10")
RULES_JSON = _env("MOCK_RULES_JSON", "")
RULES = json.loads(RULES_JSON) if RULES_JSON else {}
LOG_LEVEL = _env("MOCK_LOG_LEVEL", "INFO")

# FBL-only knobs.
FBL_REDIS_URL = _env("FBL_REDIS_URL", "redis://redis:6379/0")
FBL_STREAM = _env("FBL_STREAM", "kumo.events")
FBL_REPORTING_MTA = _env("FBL_REPORTING_MTA", "fbl.test")

logging.basicConfig(
    level=LOG_LEVEL,
    format="%(asctime)s %(levelname)s mockmta[%(name)s] %(message)s",
    stream=sys.stdout,
)
log = logging.getLogger(BEHAVIOR)

# Lazy import — only fbl mode needs redis.
_redis = None
def _get_redis():
    global _redis
    if _redis is None:
        import redis  # noqa: WPS433 — local import is intentional
        _redis = redis.Redis.from_url(FBL_REDIS_URL)
    return _redis


def _match_rule(rcpt: str) -> str | None:
    """Return the override response string for `rcpt`, or None to use BEHAVIOR."""
    for pattern, response in RULES.items():
        if fnmatch.fnmatch(rcpt, pattern):
            return response
    return None


def _publish_synthetic_feedback(envelope) -> None:
    """Publish a Feedback log_record into Redis so the consumer + DB pipeline
    sees it. Mimics the JSON shape kumomta emits when it parses an ARF report."""
    rcpt_to = envelope.rcpt_tos[0] if envelope.rcpt_tos else ""
    mail_from = envelope.mail_from or ""
    now = int(time.time())
    record = {
        "type": "Feedback",
        "id": f"mockfbl-{now}-{rcpt_to}",
        "sender": mail_from,
        "recipient": rcpt_to,
        "queue": "feedback",
        "site": "mock-mta-fbl",
        "size": len(envelope.content or b""),
        "timestamp": now,
        "created": now,
        "num_attempts": 0,
        "feedback_report": {
            "feedback_type": "abuse",
            "user_agent": "mockmta/1.0",
            "version": "1",
            "original_rcpt_to": f"<{rcpt_to}>",
            "original_mail_from": f"<{mail_from}>",
            "arrival_date": time.strftime("%a, %d %b %Y %H:%M:%S +0000", time.gmtime(now)),
            "source_ip": "203.0.113.99",
            "reported_domain": FBL_REPORTING_MTA,
        },
        "meta": {"injected_by": "mockmta-fbl"},
    }
    try:
        r = _get_redis()
        r.xadd(
            FBL_STREAM,
            {"type": "Feedback", "data": json.dumps(record)},
            maxlen=100_000,
            approximate=True,
        )
        log.info("fbl: published synthetic Feedback for %s", rcpt_to)
    except Exception as exc:  # noqa: BLE001 — surface any redis error in logs
        log.warning("fbl: redis publish failed: %s", exc)


class MockHandler:
    """aiosmtpd handler. Returns the SMTP response string from handle_DATA."""

    async def handle_RCPT(self, server, session, envelope, address, rcpt_options):
        envelope.rcpt_tos.append(address)
        return "250 OK"

    async def handle_DATA(self, server, session, envelope):
        rcpt = envelope.rcpt_tos[0] if envelope.rcpt_tos else "<unknown>"
        log.info(
            "rcvd from=%s rcpt=%s size=%d behavior=%s",
            envelope.mail_from,
            rcpt,
            len(envelope.content or b""),
            BEHAVIOR,
        )

        # Per-recipient override beats the global behavior.
        if (override := _match_rule(rcpt)):
            log.info("rule-override rcpt=%s response=%r", rcpt, override)
            return override

        if BEHAVIOR == "accept":
            return "250 2.0.0 OK message accepted"
        if BEHAVIOR == "bounce":
            return f"550 5.1.1 user unknown <{rcpt}>"
        if BEHAVIOR == "defer":
            return "450 4.7.1 try again later (mock defer)"
        if BEHAVIOR == "slow":
            log.info("slow: sleeping %ds before 250", DELAY_SEC)
            await asyncio.sleep(DELAY_SEC)
            return "250 2.0.0 OK accepted after delay"
        if BEHAVIOR == "fbl":
            _publish_synthetic_feedback(envelope)
            return "250 2.0.0 OK feedback recorded"

        log.warning("unknown MOCK_BEHAVIOR=%r — defaulting to 250", BEHAVIOR)
        return "250 OK"


def main() -> None:
    log.info(
        "starting mockmta behavior=%s delay=%ds rules=%d",
        BEHAVIOR,
        DELAY_SEC,
        len(RULES),
    )
    controller = Controller(MockHandler(), hostname="0.0.0.0", port=25)
    controller.start()
    try:
        asyncio.get_event_loop().run_forever()
    except KeyboardInterrupt:
        pass
    finally:
        controller.stop()


if __name__ == "__main__":
    main()

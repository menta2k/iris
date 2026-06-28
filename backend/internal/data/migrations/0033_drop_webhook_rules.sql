-- 0033_drop_webhook_rules.sql
-- Retire the standalone webhook subsystem. Inbound routing (inbound_routes, the
-- webhook action) now owns delivering inbound mail to an HTTP endpoint — it POSTs
-- the raw RFC822 message via kumod. The old webhook_rules drove only the
-- reception-event JSON notification worker (path B), which is removed. Existing
-- webhook_rules were already backfilled into inbound_routes by migration 0030.

-- The webhook delivery hypertable backs a stats view (migration 0003); drop it
-- first so the table drop is not blocked by the dependency.
DROP VIEW IF EXISTS webhook_stats_1h;
DROP TABLE IF EXISTS webhook_delivery_events;
DROP TABLE IF EXISTS webhook_rules;

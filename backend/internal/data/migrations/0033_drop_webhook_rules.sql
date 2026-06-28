-- 0033_drop_webhook_rules.sql
-- Retire the standalone webhook subsystem. Inbound routing (inbound_routes, the
-- webhook action) now owns delivering inbound mail to an HTTP endpoint — it POSTs
-- the raw RFC822 message via kumod. The old webhook_rules drove only the
-- reception-event JSON notification worker (path B), which is removed. Existing
-- webhook_rules were already backfilled into inbound_routes by migration 0030.

DROP TABLE IF EXISTS webhook_delivery_events;
DROP TABLE IF EXISTS webhook_rules;

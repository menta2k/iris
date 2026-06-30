-- Traffic Shaping Automation (TSA) daemon policy for Iris.
--
-- The TSA daemon receives the delivery events kumod publishes to it and computes
-- reactive shaping overrides (tighten message/connection rates, suspend on
-- repeated 4xx / deferrals) according to the automation rules in the shaping
-- config it loads. kumod subscribes to this daemon and merges those overrides
-- UNDER the IP-warmup ceiling iris renders — so warmup sets the maximum and TSA
-- can only tighten it.
--
-- Wire it up: run the backend (kumod policy generator) with
--   IRIS_TSA_URL=http://tsa-daemon:8008   (compose network)
--   IRIS_TSA_URL=http://localhost:8008     (backend on the host)
-- which makes the generated policy publish/subscribe to this daemon.

local tsa = require 'tsa'
local kumo = require 'kumo'

kumo.on('tsa_init', function()
  tsa.start_http_listener {
    listen = '0.0.0.0:8008',
    -- Who may publish events to / read overrides from the daemon. Open on the
    -- compose network for local use; TIGHTEN to the kumod host(s) in production.
    trusted_hosts = { '0.0.0.0/0' },
  }
end)

local cached_load_shaping_data = kumo.memoize(kumo.shaping.load, {
  name = 'tsa_load_shaping_data',
  ttl = '5 minutes',
  capacity = 4,
})

kumo.on('tsa_load_shaping_data', function()
  return cached_load_shaping_data {
    -- KumoMTA's community shaping config supplies the tuned per-provider
    -- automation rules (the [[provider.*.automation]] sections) that drive the
    -- back-off. Iris's blueprints don't define automation, so these are the
    -- source of the reactive logic.
    '/opt/kumomta/share/policy-extras/shaping.toml',
    -- Optional: also load iris's generated base blueprints (the kumod policy
    -- volume is mounted read-only at this path in compose). Uncomment once you
    -- have applied a config so the file exists:
    -- '/opt/kumomta/etc/policy/iris-base.toml',
  }
end)

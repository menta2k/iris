-- Gate a bounce-action rule on the message's delivery-attempt count: the rule
-- only applies once the message has been tried at least min_attempts times.
-- 0 (the default) means it applies on the first matching event. Used to suppress
-- a recipient only after repeated transient failures (e.g. a persistently-full
-- mailbox) rather than on the first deferral.
ALTER TABLE bounce_action_rules
    ADD COLUMN IF NOT EXISTS min_attempts INTEGER NOT NULL DEFAULT 0;

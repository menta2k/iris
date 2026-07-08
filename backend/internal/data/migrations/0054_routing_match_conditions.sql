-- Routing rules gain multiple match conditions (OR semantics).
--
-- A mailclass rule can now carry several header/value pairs and matches when
-- ANY of them matches. match_conditions holds the full list as
-- [{"header": "...", "value": "..."}, ...]; match_header/match_value continue to
-- mirror the first condition for backward compatibility and filtering. Existing
-- rows default to an empty array; the app synthesizes a single condition from
-- match_header/match_value when the array is empty.
ALTER TABLE routing_rules
    ADD COLUMN IF NOT EXISTS match_conditions JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Store the full raw fetched message (not just the header block) for manual
-- deliverability analysis / download as .eml. Kept out of the probe list query
-- (fetched only on demand) since a message can be large.
ALTER TABLE monitoring_probes
    ADD COLUMN IF NOT EXISTS raw_message TEXT NOT NULL DEFAULT '';

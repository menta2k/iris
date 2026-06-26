-- 0027_vmta_egress_identity.sql
-- Decouple a VMTA's outbound egress identity from its listener (reverting the
-- 0006 consolidation): the source IP and EHLO hostname move back ONTO the VMTA,
-- so a VMTA is a self-contained sending identity and a listener is purely an
-- inbound bind point. listener_id is kept as an OPTIONAL back-reference, no
-- longer the source of IP/EHLO.

ALTER TABLE vmtas ADD COLUMN IF NOT EXISTS ip_address INET;
ALTER TABLE vmtas ADD COLUMN IF NOT EXISTS ehlo_name TEXT NOT NULL DEFAULT '';

-- Backfill each existing VMTA's egress identity from its attached listener so no
-- configuration is lost on upgrade.
UPDATE vmtas v
SET ip_address = l.ip_address,
    ehlo_name  = l.hostname
FROM listeners l
WHERE l.id = v.listener_id
  AND v.ip_address IS NULL;

-- listener_id becomes an optional reference: deleting a listener no longer needs
-- to be restricted by attached VMTAs (they own their IP/EHLO now); null it out.
ALTER TABLE vmtas DROP CONSTRAINT IF EXISTS vmtas_listener_id_fkey;
ALTER TABLE vmtas
    ADD CONSTRAINT vmtas_listener_id_fkey
    FOREIGN KEY (listener_id) REFERENCES listeners(id) ON DELETE SET NULL;

-- Re-establish: only one active VMTA may bind a given IP (as before 0006).
CREATE UNIQUE INDEX IF NOT EXISTS vmtas_active_ip_uniq
    ON vmtas (ip_address) WHERE status = 'active';

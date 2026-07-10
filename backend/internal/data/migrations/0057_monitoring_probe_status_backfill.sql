-- Backfill probes written before mailbox_status/send_status were set explicitly.
-- An empty status was inserted over the column DEFAULT, so the reconciler/fetch
-- selectors (which filter on = 'queued' / = 'pending') never matched them.
UPDATE monitoring_probes SET send_status = 'queued' WHERE send_status = '';
UPDATE monitoring_probes SET mailbox_status = 'skipped'
    WHERE mailbox_status = '' AND send_status = 'error';
UPDATE monitoring_probes SET mailbox_status = 'pending'
    WHERE mailbox_status = '';

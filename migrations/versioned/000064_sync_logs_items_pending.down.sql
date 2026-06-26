-- Reverse of 000064: drop the items_pending counter from sync_logs.
ALTER TABLE sync_logs DROP COLUMN IF EXISTS items_pending;

-- Add items_pending to sync_logs for DingTalk ALIDOC async ingest (phase 2).
-- 000063 created dingtalk_doc_pending but missed the counter column on
-- sync_logs that the Stream listener decrements as content arrives.
DO $$ BEGIN RAISE NOTICE '[Migration 000064] Adding sync_logs.items_pending...'; END $$;

ALTER TABLE sync_logs ADD COLUMN IF NOT EXISTS items_pending INTEGER NOT NULL DEFAULT 0;

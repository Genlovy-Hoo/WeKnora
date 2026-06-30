-- DingTalk online document (ALIDOC) async ingest support (phase 2).
-- ALIDOC content can only be obtained via the DingTalk Stream event bus:
-- the connector triggers an export (query_doc_content → taskId), records a
-- pending row here, and a long-lived Stream listener receives the
-- doc_content_export_result event carrying the markdown content, then ingests
-- it via CreateKnowledgeFromFile and marks the row done.
DO $$ BEGIN RAISE NOTICE '[Migration 000063] Creating dingtalk_doc_pending...'; END $$;

CREATE TABLE IF NOT EXISTS dingtalk_doc_pending (
    id              VARCHAR(36) PRIMARY KEY,
    tenant_id       BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    datasource_id   VARCHAR(36) NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    sync_log_id     VARCHAR(36),
    source_resource_id VARCHAR(256) NOT NULL DEFAULT '',
    node_id         VARCHAR(128) NOT NULL,
    doc_url         VARCHAR(1024) NOT NULL,
    title           VARCHAR(512) NOT NULL DEFAULT '',
    extension       VARCHAR(32) NOT NULL DEFAULT '',
    task_id         VARCHAR(128) NOT NULL DEFAULT '',
    status          VARCHAR(32) NOT NULL DEFAULT 'pending',  -- pending | done | failed
    error_message   TEXT NOT NULL DEFAULT '',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- One active pending per (datasource, node): a re-trigger (incremental sync
-- of an edited doc) replaces the previous pending row so the Stream callback
-- matches by doc_url and never ingests a stale version.
CREATE UNIQUE INDEX IF NOT EXISTS idx_dingtalk_pending_ds_node
    ON dingtalk_doc_pending(datasource_id, node_id)
    WHERE status = 'pending';

-- Stream callback matches pending rows by doc_url; index for fast lookup.
CREATE INDEX IF NOT EXISTS idx_dingtalk_pending_url
    ON dingtalk_doc_pending(doc_url)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_dingtalk_pending_ds
    ON dingtalk_doc_pending(datasource_id);

CREATE INDEX IF NOT EXISTS idx_dingtalk_pending_synclog
    ON dingtalk_doc_pending(sync_log_id);

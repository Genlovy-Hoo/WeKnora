-- Migration 000065: Custom tool libraries and HTTP tools
-- Makes tool management a first-class citizen (like knowledge bases / skills).
-- tool_libraries: groups of tools (builtin library is virtual/seeded per tenant).
-- custom_tools:   user-registered HTTP API tools, registered into the agent
--                  ToolRegistry alongside builtin tools via allowed_tools.
DO $$ BEGIN RAISE NOTICE '[Migration 000065] Creating tool_libraries / custom_tools...'; END $$;

CREATE TABLE IF NOT EXISTS tool_libraries (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    is_builtin BOOLEAN NOT NULL DEFAULT false,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_libraries_tenant_name
    ON tool_libraries(tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_tool_libraries_tenant ON tool_libraries(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_tool_libraries_is_builtin ON tool_libraries(is_builtin);

COMMENT ON TABLE tool_libraries IS 'Tool libraries (groups) — first-class tool management';

CREATE TABLE IF NOT EXISTS custom_tools (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    library_id VARCHAR(36) NOT NULL REFERENCES tool_libraries(id) ON DELETE CASCADE,
    name VARCHAR(64) NOT NULL,
    display_name VARCHAR(128),
    description TEXT NOT NULL,
    parameters_schema JSONB,
    http_config JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    require_approval BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_tools_tenant_name
    ON custom_tools(tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_custom_tools_tenant_lib
    ON custom_tools(tenant_id, library_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_custom_tools_enabled ON custom_tools(enabled);

COMMENT ON TABLE custom_tools IS 'User-registered HTTP API tools (non-MCP)';

-- updated_at triggers (mirrors mcp_services pattern)
CREATE OR REPLACE FUNCTION update_tool_libraries_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trigger_tool_libraries_updated_at') THEN
        CREATE TRIGGER trigger_tool_libraries_updated_at
            BEFORE UPDATE ON tool_libraries
            FOR EACH ROW EXECUTE FUNCTION update_tool_libraries_updated_at();
    END IF;
END $$;

CREATE OR REPLACE FUNCTION update_custom_tools_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trigger_custom_tools_updated_at') THEN
        CREATE TRIGGER trigger_custom_tools_updated_at
            BEFORE UPDATE ON custom_tools
            FOR EACH ROW EXECUTE FUNCTION update_custom_tools_updated_at();
    END IF;
END $$;

DO $$ BEGIN RAISE NOTICE '[Migration 000065] tool_libraries / custom_tools ready'; END $$;

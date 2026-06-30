-- Rollback Migration 000065: drop custom tools tables
DO $$ BEGIN RAISE NOTICE '[Migration 000065 rollback] Dropping custom_tools / tool_libraries...'; END $$;

DROP TRIGGER IF EXISTS trigger_custom_tools_updated_at ON custom_tools;
DROP FUNCTION IF EXISTS update_custom_tools_updated_at();
DROP TABLE IF EXISTS custom_tools;

DROP TRIGGER IF EXISTS trigger_tool_libraries_updated_at ON tool_libraries;
DROP FUNCTION IF EXISTS update_tool_libraries_updated_at();
DROP TABLE IF EXISTS tool_libraries;

DO $$ BEGIN RAISE NOTICE '[Migration 000065 rollback] done'; END $$;

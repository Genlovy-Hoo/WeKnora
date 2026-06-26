package stream

import (
	"context"
	"mime/multipart"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// pendingDDL mirrors migrations/versioned/000063 (sqlite-flavoured).
const pendingDDL = `
CREATE TABLE dingtalk_doc_pending (
    id              TEXT PRIMARY KEY,
    tenant_id       INTEGER NOT NULL,
    knowledge_base_id TEXT NOT NULL,
    datasource_id   TEXT NOT NULL,
    sync_log_id     TEXT,
    source_resource_id TEXT NOT NULL DEFAULT '',
    node_id         TEXT NOT NULL,
    doc_url         TEXT NOT NULL,
    title           TEXT NOT NULL DEFAULT '',
    extension       TEXT NOT NULL DEFAULT '',
    task_id         TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'pending',
    error_message   TEXT NOT NULL DEFAULT '',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE sync_logs (
    id              TEXT PRIMARY KEY,
    items_pending   INTEGER DEFAULT 0,
    items_created   INTEGER DEFAULT 0,
    items_failed    INTEGER DEFAULT 0,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

// fakeIngester records calls to CreateKnowledgeFromFile for assertions.
type fakeIngester struct {
	calls    int
	lastKB   string
	lastFN   string
	lastMeta map[string]string
	fail     error
}

func (f *fakeIngester) CreateKnowledgeFromFile(
	_ context.Context, kbID string, _ *multipart.FileHeader, metadata map[string]string,
	_ *bool, customFileName string, _ []string, _ string, _ *types.KnowledgeProcessOverrides,
) (*types.Knowledge, error) {
	f.calls++
	f.lastKB = kbID
	f.lastFN = customFileName
	f.lastMeta = metadata
	if f.fail != nil {
		return nil, f.fail
	}
	return &types.Knowledge{ID: "k1"}, nil
}

func setupListenerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(pendingDDL).Error; err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return db
}

func insertPending(t *testing.T, db *gorm.DB, id, url, syncLogID, sourceResourceID string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO dingtalk_doc_pending
		(id, tenant_id, knowledge_base_id, datasource_id, sync_log_id, source_resource_id, node_id, doc_url, title, extension, status)
		VALUES (?, 10000, 'kb1', 'ds1', ?, ?, ?, ?, 'doc.adoc', 'adoc', 'pending')`,
		id, syncLogID, sourceResourceID, url, url).Error; err != nil {
		t.Fatalf("insert pending: %v", err)
	}
}

func insertSyncLog(t *testing.T, db *gorm.DB, id string, pending int) {
	t.Helper()
	if err := db.Exec(`INSERT INTO sync_logs (id, items_pending, items_created, items_failed) VALUES (?, ?, 0, 0)`,
		id, pending).Error; err != nil {
		t.Fatalf("insert sync_log: %v", err)
	}
}

func TestIngestFromEventMatchesPendingAndMarksDone(t *testing.T) {
	db := setupListenerDB(t)
	ing := &fakeIngester{}
	l := NewListener(db)
	l.SetIngester(ing)

	insertSyncLog(t, db, "log1", 1)
	insertPending(t, db, "p1", "https://alidocs.dingtalk.com/i/nodes/n2", "log1", "node:n2")

	ev := &docContentExportEvent{
		Success: true,
		URL:     "https://alidocs.dingtalk.com/i/nodes/n2",
		Name:    "doc.adoc",
		Content: "# title\n\nbody",
		Format:  "markdown",
	}
	if err := l.ingestFromEvent(context.Background(), ev); err != nil {
		t.Fatalf("ingestFromEvent: %v", err)
	}

	// Ingester was called with the pending row's KB and external_id metadata.
	if ing.calls != 1 {
		t.Fatalf("expected 1 ingest call, got %d", ing.calls)
	}
	if ing.lastKB != "kb1" {
		t.Errorf("expected kb1, got %s", ing.lastKB)
	}
	if ing.lastMeta["external_id"] == "" {
		t.Error("expected external_id in metadata")
	}
	if ing.lastMeta["source_resource_id"] != "node:n2" {
		t.Errorf("expected source_resource_id=node:n2, got %q", ing.lastMeta["source_resource_id"])
	}
	if ing.lastFN != "doc.adoc.md" {
		t.Errorf("expected doc.adoc.md, got %s", ing.lastFN)
	}

	// Pending row marked done.
	var p types.DingTalkDocPending
	if err := db.First(&p, "id = ?", "p1").Error; err != nil {
		t.Fatalf("load pending: %v", err)
	}
	if p.Status != types.DingTalkPendingStatusDone {
		t.Errorf("expected done, got %s", p.Status)
	}

	// Sync log counters bumped: pending--, created++.
	var sl struct {
		ItemsPending int
		ItemsCreated int
	}
	if err := db.Table("sync_logs").Where("id = ?", "log1").Scan(&sl).Error; err != nil {
		t.Fatalf("load sync_log: %v", err)
	}
	if sl.ItemsPending != 0 {
		t.Errorf("expected pending=0, got %d", sl.ItemsPending)
	}
	if sl.ItemsCreated != 1 {
		t.Errorf("expected created=1, got %d", sl.ItemsCreated)
	}
}

func TestIngestFromEventNoPendingRowIsNoop(t *testing.T) {
	db := setupListenerDB(t)
	ing := &fakeIngester{}
	l := NewListener(db)
	l.SetIngester(ing)

	// No pending row for this url.
	ev := &docContentExportEvent{
		Success: true,
		URL:     "https://alidocs.dingtalk.com/i/nodes/unknown",
		Content: "body",
	}
	if err := l.ingestFromEvent(context.Background(), ev); err != nil {
		t.Fatalf("ingestFromEvent: %v", err)
	}
	if ing.calls != 0 {
		t.Errorf("expected 0 ingest calls for unmatched url, got %d", ing.calls)
	}
}

func TestIngestFromEventFailureMarksPendingFailed(t *testing.T) {
	db := setupListenerDB(t)
	ing := &fakeIngester{fail: errFailed}
	l := NewListener(db)
	l.SetIngester(ing)

	insertSyncLog(t, db, "log1", 1)
	insertPending(t, db, "p1", "https://alidocs.dingtalk.com/i/nodes/n2", "log1", "node:n2")

	ev := &docContentExportEvent{
		Success: true,
		URL:     "https://alidocs.dingtalk.com/i/nodes/n2",
		Content: "body",
	}
	_ = l.ingestFromEvent(context.Background(), ev)

	var p types.DingTalkDocPending
	if err := db.First(&p, "id = ?", "p1").Error; err != nil {
		t.Fatalf("load pending: %v", err)
	}
	if p.Status != types.DingTalkPendingStatusFailed {
		t.Errorf("expected failed, got %s", p.Status)
	}
	if p.ErrorMessage == "" {
		t.Error("expected error_message to be set")
	}

	var sl struct {
		ItemsPending int
		ItemsFailed  int
	}
	if err := db.Table("sync_logs").Where("id = ?", "log1").Scan(&sl).Error; err != nil {
		t.Fatalf("load sync_log: %v", err)
	}
	if sl.ItemsPending != 0 {
		t.Errorf("expected pending=0 after failure bump, got %d", sl.ItemsPending)
	}
	if sl.ItemsFailed != 1 {
		t.Errorf("expected failed=1, got %d", sl.ItemsFailed)
	}
}

// errFailed is a sentinel for the fakeIngester failure path.
var errFailed = &simpleErr{"ingest failed"}

type simpleErr struct{ msg string }

func (e *simpleErr) Error() string { return e.msg }

// ensure time is referenced (used in DDL defaults; keeps import tidy).
var _ = time.Now

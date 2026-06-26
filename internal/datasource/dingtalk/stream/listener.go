// Package stream implements the DingTalk Stream event listener that completes
// the async ingest of ALIDOC online documents.
//
// DingTalk's doc-content APIs return a taskId and deliver the actual markdown
// body asynchronously via the Stream doc_content_export_result event. The
// connector (connector.go) triggers the export and records a pending row in
// dingtalk_doc_pending; this listener receives the event, matches it to a
// pending row by DocURL, ingests the content via CreateKnowledgeFromFile, and
// marks the row done.
//
// One stream connection per active DingTalk data source (each has its own
// AppKey/AppSecret). Connections are managed dynamically as data sources are
// created/deleted, mirroring the IM long-conn pattern.
package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	dtc "github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
	"gorm.io/gorm"
)

// KnowledgeIngester is the subset of KnowledgeService the listener needs to
// ingest a fetched markdown body. Kept narrow to avoid pulling the full
// interface and to keep the listener testable.
type KnowledgeIngester interface {
	// CreateKnowledgeFromFile ingests a file. metadata carries external_id etc.
	CreateKnowledgeFromFile(
		ctx context.Context, kbID string, file *multipart.FileHeader, metadata map[string]string,
		enableMultimodel *bool, customFileName string, tagIDs []string, channel string,
		processOverrides *types.KnowledgeProcessOverrides,
	) (*types.Knowledge, error)
}

// Listener manages stream connections for all active DingTalk data sources and
// ingests ALIDOC content as doc_content_export_result events arrive.
type Listener struct {
	db       *gorm.DB
	ingester KnowledgeIngester

	mu      sync.Mutex
	clients map[string]*managedClient // keyed by dataSourceID
}

type managedClient struct {
	dsID      string
	appKey    string
	appSecret string
	cli       *client.StreamClient
	cancel    context.CancelFunc
}

// NewListener creates a new DingTalk stream listener. The ingester is bound
// later via SetIngester to avoid a dig construction cycle (KnowledgeService
// depends on repos that depend on the db; the listener also depends on the db).
func NewListener(db *gorm.DB) *Listener {
	return &Listener{
		db:      db,
		clients: make(map[string]*managedClient),
	}
}

// SetIngester binds the knowledge ingester used to write ALIDOC content into
// the knowledge base. Called once at startup after construction.
func (l *Listener) SetIngester(ingester KnowledgeIngester) { l.ingester = ingester }

// Start loads all active DingTalk data sources, decrypts their credentials,
// and opens a stream connection for each. Invoked once at app startup (see
// container). Reconnecting here is required: scheduled syncs trigger ALIDOC
// exports whose markdown body arrives on the stream, so after a restart the
// stream must be back up before any sync runs.
func (l *Listener) Start(ctx context.Context) error {
	rows, err := l.db.Table("data_sources").
		Select("id, config").
		Where("type = ? AND status = ? AND deleted_at IS NULL",
			types.ConnectorTypeDingTalk, types.DataSourceStatusActive).Rows()
	if err != nil {
		return fmt.Errorf("load dingtalk data sources: %w", err)
	}
	defer rows.Close()
	connected := 0
	for rows.Next() {
		var id string
		var configBlob []byte
		if err := rows.Scan(&id, &configBlob); err != nil {
			logger.Warnf(ctx, "[DingTalk-Stream] scan row failed: %v", err)
			continue
		}
		cfg, err := (&types.DataSource{ID: id, Config: types.JSON(configBlob)}).ParseConfig()
		if err != nil || cfg == nil {
			logger.Warnf(ctx, "[DingTalk-Stream] parse config failed for ds=%s: %v", id, err)
			continue
		}
		appKey, _ := cfg.Credentials["app_key"].(string)
		appSecret, _ := cfg.Credentials["app_secret"].(string)
		if appKey == "" || appSecret == "" {
			logger.Warnf(ctx, "[DingTalk-Stream] no credentials for ds=%s, skipping", id)
			continue
		}
		l.EnsureDataSource(ctx, id, appKey, appSecret)
		connected++
	}
	logger.Infof(ctx, "[DingTalk-Stream] started listener, connected %d active data source(s)", connected)
	return nil
}

// EnsureDataSource opens (or replaces) the stream connection for a data source
// using the provided decrypted credentials. Called by the data-source service
// on create/update/resume.
func (l *Listener) EnsureDataSource(ctx context.Context, dsID, appKey, appSecret string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// If a client for this ds already exists with the same credentials, skip.
	if existing, ok := l.clients[dsID]; ok && existing.appKey == appKey {
		return
	}
	// Close any existing client for this ds (credentials changed).
	if existing, ok := l.clients[dsID]; ok {
		existing.cancel()
		existing.cli.Close()
		delete(l.clients, dsID)
	}

	cli := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(appKey, appSecret)),
	)
	cli.RegisterAllEventRouter(l.onEvent)

	connCtx, cancel := context.WithCancel(ctx)
	mc := &managedClient{dsID: dsID, appKey: appKey, appSecret: appSecret, cli: cli, cancel: cancel}
	l.clients[dsID] = mc

	go func() {
		logger.Infof(connCtx, "[DingTalk-Stream] connecting stream for ds=%s", dsID)
		if err := cli.Start(connCtx); err != nil {
			logger.Errorf(connCtx, "[DingTalk-Stream] stream ended for ds=%s: %v", dsID, err)
		}
	}()
}

// RemoveDataSource closes the stream connection for a data source (on
// pause/delete).
func (l *Listener) RemoveDataSource(ctx context.Context, dsID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if mc, ok := l.clients[dsID]; ok {
		mc.cancel()
		mc.cli.Close()
		delete(l.clients, dsID)
		logger.Infof(ctx, "[DingTalk-Stream] disconnected stream for ds=%s", dsID)
	}
}

// Stop closes all stream connections. Registered with ResourceCleaner at startup.
func (l *Listener) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for dsID, mc := range l.clients {
		mc.cancel()
		mc.cli.Close()
		delete(l.clients, dsID)
	}
	return nil
}

// docContentExportEvent is the payload of the doc_content_export_result event.
// Fields mirror llmflow's listener (event.data): url matches pending.DocURL.
type docContentExportEvent struct {
	Success   bool   `json:"success"`
	URL       string `json:"url"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	Format    string `json:"format"`
	Content   string `json:"content"`
}

// onEvent is the registered handler for all stream events; it only acts on
// doc_content_export_result.
func (l *Listener) onEvent(ctx context.Context, df *payload.DataFrame) (*payload.DataFrameResponse, error) {
	header := dtc.NewEventHeaderFromDataFrame(df)
	if header.EventType != "doc_content_export_result" {
		return payload.NewSuccessDataFrameResponse(), nil
	}

	var ev docContentExportEvent
	if err := json.Unmarshal([]byte(df.Data), &ev); err != nil {
		logger.Errorf(ctx, "[DingTalk-Stream] decode event failed: %v", err)
		return payload.NewSuccessDataFrameResponse(), nil
	}
	if !ev.Success || ev.URL == "" {
		logger.Warnf(ctx, "[DingTalk-Stream] event not successful or no url: %+v", ev)
		return payload.NewSuccessDataFrameResponse(), nil
	}

	if err := l.ingestFromEvent(ctx, &ev); err != nil {
		logger.Errorf(ctx, "[DingTalk-Stream] ingest failed for url=%s: %v", ev.URL, err)
	}
	return payload.NewSuccessDataFrameResponse(), nil
}

// ingestFromEvent matches the event to a pending row by DocURL, ingests the
// markdown content, and updates the row + sync-log counters.
func (l *Listener) ingestFromEvent(ctx context.Context, ev *docContentExportEvent) error {
	var pending types.DingTalkDocPending
	// Match the oldest pending row for this url (a doc may have been
	// re-triggered; the unique index keeps only the latest pending).
	err := l.db.Where("doc_url = ? AND status = ?", ev.URL, types.DingTalkPendingStatusPending).
		Order("created_at ASC").First(&pending).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No pending row — either already ingested, or the doc was synced
			// via a path that doesn't use pending (e.g. manual). Ignore.
			logger.Infof(ctx, "[DingTalk-Stream] no pending row for url=%s, skipping", ev.URL)
			return nil
		}
		return fmt.Errorf("find pending: %w", err)
	}

	// Build a markdown file from the content and ingest it.
	fileName := ev.Name
	if fileName == "" {
		fileName = pending.Title
	}
	if fileName == "" {
		fileName = pending.NodeID
	}
	if !endsWithExt(fileName, "md") {
		fileName = fileName + ".md"
	}

	fh, err := bytesToFileHeader([]byte(ev.Content), fileName)
	if err != nil {
		return fmt.Errorf("build file header: %w", err)
	}
	sourceResourceID := pending.SourceResourceID
	if sourceResourceID == "" {
		sourceResourceID = "node:" + pending.NodeID
	}
	meta := map[string]string{
		"external_id":        pending.NodeID,
		"source_resource_id": sourceResourceID,
		"datasource_id":      pending.DataSourceID,
		"node_id":            pending.NodeID,
		"doc_url":            ev.URL,
		"channel":            types.ChannelDingtalk,
	}

	// Resolve the auto-tag the connector would have used (same name as ds).
	// ponytail: tag lookup skipped; CreateKnowledgeFromFile accepts empty
	// tagIDs (no tag). Tagging can be backfilled by a later sync if needed.

	tenantCtx := withTenant(ctx, pending.TenantID)
	if _, err := l.ingester.CreateKnowledgeFromFile(
		tenantCtx, pending.KnowledgeBaseID, fh, meta, nil, fileName, nil,
		types.ChannelDingtalk, nil,
	); err != nil {
		// Mark failed but keep the row so a housekeeping sweep can retry.
		l.db.Model(&pending).Updates(map[string]interface{}{
			"status":        types.DingTalkPendingStatusFailed,
			"error_message": err.Error(),
			"updated_at":    time.Now(),
		})
		l.bumpSyncLog(ctx, pending.SyncLogID, pending.TenantID, false)
		return fmt.Errorf("create knowledge from file: %w", err)
	}

	// Success: mark done and decrement the sync-log pending counter.
	now := time.Now()
	if err := l.db.Model(&pending).Updates(map[string]interface{}{
		"status":     types.DingTalkPendingStatusDone,
		"updated_at": now,
	}).Error; err != nil {
		logger.Warnf(ctx, "[DingTalk-Stream] failed to mark pending done: %v", err)
	}
	l.bumpSyncLog(ctx, pending.SyncLogID, pending.TenantID, true)
	return nil
}

// bumpSyncLog updates a sync log's counters after an async ingest: decrements
// items_pending and increments items_created (or items_failed).
func (l *Listener) bumpSyncLog(ctx context.Context, syncLogID string, tenantID uint64, success bool) {
	if syncLogID == "" {
		return
	}
	updates := map[string]interface{}{"updated_at": time.Now()}
	if success {
		updates["items_pending"] = gorm.Expr("CASE WHEN items_pending > 0 THEN items_pending - 1 ELSE 0 END")
		updates["items_created"] = gorm.Expr("items_created + 1")
	} else {
		updates["items_pending"] = gorm.Expr("CASE WHEN items_pending > 0 THEN items_pending - 1 ELSE 0 END")
		updates["items_failed"] = gorm.Expr("items_failed + 1")
	}
	if err := l.db.Table("sync_logs").Where("id = ?", syncLogID).Updates(updates).Error; err != nil {
		logger.Warnf(ctx, "[DingTalk-Stream] failed to bump sync log %s: %v", syncLogID, err)
	}
}

// withTenant returns a context carrying the tenant id, so CreateKnowledgeFromFile
// can read tenant-scoped KB config. The real tenant context key is types.TenantIDContextKey.
func withTenant(ctx context.Context, tenantID uint64) context.Context {
	return context.WithValue(ctx, types.TenantIDContextKey, tenantID)
}

// --- small helpers ---

func endsWithExt(name, ext string) bool {
	return len(name) > len(ext)+1 &&
		name[len(name)-len(ext)-1] == '.' &&
		eqIgnoreCase(name[len(name)-len(ext):], ext)
}

func eqIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// bytesToFileHeader wraps a []byte into a *multipart.FileHeader so it can be
// consumed by KnowledgeService.CreateKnowledgeFromFile. Mirrors the helper in
// datasource_service.go.
func bytesToFileHeader(data []byte, filename string) (*multipart.FileHeader, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	partHeader.Set("Content-Type", "application/octet-stream")
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return nil, fmt.Errorf("create multipart part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("write data to part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}
	reader := multipart.NewReader(&buf, writer.Boundary())
	form, err := reader.ReadForm(int64(len(data)) + 1024)
	if err != nil {
		return nil, fmt.Errorf("read multipart form: %w", err)
	}
	files := form.File["file"]
	if len(files) == 0 {
		return nil, fmt.Errorf("no file in multipart form")
	}
	return files[0], nil
}

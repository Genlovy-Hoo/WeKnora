package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// fakeDingTalk builds an httptest.Server emulating the DingTalk APIs the
// connector uses: auth, user→unionId, GetWorkspace, ListNodes,
// queryDentryId, downloadInfos, and the signed file GET.
//
// It serves a tiny tree: workspace ws1 → root → [file.pdf (DOCUMENT), doc.adoc (ALIDOC, skipped)].
func fakeDingTalk(t *testing.T) (*httptest.Server, *Config) {
	t.Helper()
	mux := http.NewServeMux()

	// /v1.0/oauth2/accessToken — note: uses api.dingtalk.com host which the
	// client hardcodes for auth, so we cannot redirect it. Instead the test
	// pre-seeds the token by calling Ping against a server that handles it.
	// To keep the test hermetic, we point the client base URL at this server
	// AND override the auth/user hosts by serving them here too under /__auth.
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"accessToken": "fake-token", "expireIn": 7200})
	})

	mux.HandleFunc("/topapi/v2/user/get", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"errcode": 0,
			"errmsg":  "ok",
			"result":  map[string]interface{}{"unionid": "union-fake", "name": "Tester"},
		})
	})

	mux.HandleFunc("/v2.0/wiki/mineWorkspaces", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"workspace": map[string]interface{}{
				"workspaceId": "ws1", "name": "My Space", "rootNodeId": "root1", "type": "PERSONAL",
			},
		})
	})

	mux.HandleFunc("/v2.0/wiki/workspaces/ws1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"workspace": map[string]interface{}{
				"workspaceId": "ws1", "name": "My Space", "rootNodeId": "root1",
			},
		})
	})

	mux.HandleFunc("/v2.0/wiki/nodes", func(w http.ResponseWriter, r *http.Request) {
		// Distinguish by parentNodeId so the nodeId fallback path can be tested.
		parent := r.URL.Query().Get("parentNodeId")
		var nodes []map[string]interface{}
		switch parent {
		case "folder1":
			nodes = []map[string]interface{}{
				{"nodeId": "n3", "name": "sub.pdf", "category": "DOCUMENT", "extension": "pdf",
					"type": "FILE", "modifiedTime": "2026-06-24T15:00Z", "workspaceId": "ws1"},
			}
		default: // root1 (workspace root) and any other
			nodes = []map[string]interface{}{
				{"nodeId": "n1", "name": "file.pdf", "category": "DOCUMENT", "extension": "pdf",
					"type": "FILE", "modifiedTime": "2026-06-24T15:00Z", "workspaceId": "ws1"},
				{"nodeId": "n2", "name": "doc.adoc", "category": "ALIDOC", "extension": "adoc",
					"type": "FILE", "modifiedTime": "2026-06-24T15:00Z", "workspaceId": "ws1"},
			}
		}
		writeJSON(w, map[string]interface{}{"nodes": nodes})
	})

	// GetNode by id — used by the nodeId fallback path.
	mux.HandleFunc("/v2.0/wiki/nodes/folder1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"node": map[string]interface{}{
				"nodeId": "folder1", "name": "My Folder", "category": "OTHER",
				"type": "FOLDER", "hasChildren": true, "workspaceId": "ws1",
				"modifiedTime": "2026-06-24T15:00Z",
			},
		})
	})
	// Any other /v2.0/wiki/nodes/{id} GetNode returns 404 so the workspace path
	// fails fast and the fallback kicks in only for real node ids.
	mux.HandleFunc("/v2.0/wiki/nodes/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v2.0/wiki/nodes/")
		if id == "folder1" || id == "" {
			return // handled by the specific handler above
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/v2.0/doc/dentries/n1/queryDentryId", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"dentryId": "d1", "spaceId": "s1", "dentryUuid": "n1"})
	})
	mux.HandleFunc("/v2.0/doc/dentries/n3/queryDentryId", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"dentryId": "d3", "spaceId": "s3", "dentryUuid": "n3"})
	})

	// The signed file GET — returns fake PDF bytes.
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("%PDF-1.4 fake pdf content"))
	})

	srv := httptest.NewServer(mux)

	// Register the downloadInfos handler after srv exists so it can reference
	// srv.URL for the signed resource URL.
	mux.HandleFunc("/v1.0/storage/spaces/s1/dentries/d1/downloadInfos/query", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"protocol": "HEADER_SIGNATURE",
			"headerSignatureInfo": map[string]interface{}{
				"headers":      map[string]interface{}{"Authorization": "OSS test"},
				"resourceUrls": []string{srv.URL + "/file"},
			},
		})
	})
	mux.HandleFunc("/v1.0/storage/spaces/s3/dentries/d3/downloadInfos/query", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"protocol": "HEADER_SIGNATURE",
			"headerSignatureInfo": map[string]interface{}{
				"headers":      map[string]interface{}{"Authorization": "OSS test"},
				"resourceUrls": []string{srv.URL + "/file"},
			},
		})
	})

	cfg := &Config{
		AppKey:    "k",
		AppSecret: "s",
		UserID:    "u1",
		BaseURL:   srv.URL,
	}
	return srv, cfg
}

func TestValidate(t *testing.T) {
	srv, cfg := fakeDingTalk(t)
	defer srv.Close()

	conn := NewConnector()
	dsCfg := &types.DataSourceConfig{
		Type: types.ConnectorTypeDingTalk,
		Credentials: map[string]interface{}{
			"app_key":    cfg.AppKey,
			"app_secret": cfg.AppSecret,
			"user_id":    cfg.UserID,
			"base_url":   cfg.BaseURL, // point at the mock server
		},
	}
	if err := conn.Validate(context.Background(), dsCfg); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestFetchAllDownloadsUploadedFileAndSkipsALIDOC(t *testing.T) {
	srv, cfg := fakeDingTalk(t)
	defer srv.Close()

	conn := NewConnector()
	dsCfg := &types.DataSourceConfig{
		Type: types.ConnectorTypeDingTalk,
		Credentials: map[string]interface{}{
			"app_key":    cfg.AppKey,
			"app_secret": cfg.AppSecret,
			"user_id":    cfg.UserID,
			"base_url":   cfg.BaseURL,
		},
		ResourceIDs: []string{"ws1"},
	}

	items, err := conn.FetchAll(context.Background(), dsCfg, []string{"ws1"})
	if err != nil {
		t.Fatalf("FetchAll failed: %v", err)
	}

	// Only the DOCUMENT file should produce an item; ALIDOC is skipped.
	if len(items) != 1 {
		t.Fatalf("expected 1 item (DOCUMENT only, ALIDOC skipped), got %d: %+v", len(items), items)
	}
	item := items[0]
	if item.ExternalID != "n1" {
		t.Errorf("expected n1, got %s", item.ExternalID)
	}
	if string(item.Content[:8]) != "%PDF-1.4" {
		t.Errorf("expected PDF magic, got %q", item.Content[:8])
	}
	if item.FileName != "file.pdf" {
		t.Errorf("expected file.pdf, got %s", item.FileName)
	}
	if item.Metadata["channel"] != types.ChannelDingtalk {
		t.Errorf("expected dingtalk channel, got %s", item.Metadata["channel"])
	}
}

func TestFetchIncrementalDetectsChanges(t *testing.T) {
	srv, cfg := fakeDingTalk(t)
	defer srv.Close()

	conn := NewConnector()
	dsCfg := &types.DataSourceConfig{
		Type: types.ConnectorTypeDingTalk,
		Credentials: map[string]interface{}{
			"app_key":    cfg.AppKey,
			"app_secret": cfg.AppSecret,
			"user_id":    cfg.UserID,
			"base_url":   cfg.BaseURL,
		},
		ResourceIDs: []string{"ws1"},
	}

	// First sync: n1 is new, n2 (ALIDOC) skipped → 1 item.
	items, cursor, err := conn.FetchIncremental(context.Background(), dsCfg, nil)
	if err != nil {
		t.Fatalf("first FetchIncremental: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("first sync: expected 1 item, got %d", len(items))
	}
	if cursor == nil {
		t.Fatal("expected non-nil cursor")
	}

	// Second sync with the same tree → no changes (modifiedTime unchanged).
	items2, _, err := conn.FetchIncremental(context.Background(), dsCfg, cursor)
	if err != nil {
		t.Fatalf("second FetchIncremental: %v", err)
	}
	if len(items2) != 0 {
		t.Fatalf("second sync: expected 0 changes, got %d", len(items2))
	}
}

// TestFetchAllWithNodeIdResourceID covers the regression where the user
// selects a node (not a whole workspace): the resource id is a nodeId, which
// makes GetWorkspace fail with paramError. The connector must fall back to
// treating it as a node and sync that subtree.
func TestFetchAllWithNodeIdResourceID(t *testing.T) {
	srv, cfg := fakeDingTalk(t)
	defer srv.Close()

	conn := NewConnector()
	dsCfg := &types.DataSourceConfig{
		Type: types.ConnectorTypeDingTalk,
		Credentials: map[string]interface{}{
			"app_key": cfg.AppKey, "app_secret": cfg.AppSecret,
			"user_id": cfg.UserID, "base_url": cfg.BaseURL,
		},
	}

	// "folder1" is a nodeId (not a workspaceId). GetWorkspace("folder1") is not
	// registered on the mock, so it 404s; the connector must fall back to
	// GetNode("folder1") and walk its children (sub.pdf).
	items, err := conn.FetchAll(context.Background(), dsCfg, []string{"folder1"})
	if err != nil {
		t.Fatalf("FetchAll with nodeId resource failed: %v", err)
	}
	if len(items) != 1 || items[0].ExternalID != "n3" {
		t.Fatalf("expected 1 item (sub.pdf under folder1), got %+v", items)
	}
	if string(items[0].Content[:8]) != "%PDF-1.4" {
		t.Errorf("expected PDF content, got %q", items[0].Content[:8])
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

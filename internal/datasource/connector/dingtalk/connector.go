package dingtalk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// Connector implements the datasource.Connector interface for DingTalk.
//
// Scope (MVP): syncs UPLOADED FILES only (category DOCUMENT/IMAGE/VIDEO/...).
// DingTalk online docs (category ALIDOC: .adoc/.axls/.able) are skipped —
// their content is only obtainable asynchronously via the Stream event bus,
// which doesn't fit the Connector's request/response contract. See package
// docs and the DingTalk sync design notes for the rationale and the phase-2
// plan for ALIDOC support.
type Connector struct{}

// NewConnector creates a new DingTalk connector.
func NewConnector() *Connector { return &Connector{} }

// Type returns the connector type identifier.
func (c *Connector) Type() string { return types.ConnectorTypeDingTalk }

// Validate verifies the DingTalk configuration by testing connectivity.
func (c *Connector) Validate(ctx context.Context, config *types.DataSourceConfig) error {
	cfg, err := parseDingTalkConfig(config)
	if err != nil {
		return err
	}
	client := NewClient(cfg)
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("dingtalk connection failed: %w", err)
	}
	return nil
}

// ListResources lists DingTalk knowledge bases (workspaces) for selection.
//
// parentID == "" → the operator's personal workspace (via mineWorkspaces).
// parentID == workspaceID → the direct children of that workspace's root node.
// This mirrors the Feishu connector's lazy one-level-at-a-time loading.
func (c *Connector) ListResources(
	ctx context.Context, config *types.DataSourceConfig, parentID string,
) ([]types.Resource, error) {
	cfg, err := parseDingTalkConfig(config)
	if err != nil {
		return nil, err
	}
	client := NewClient(cfg)

	if parentID == "" {
		spaces, err := client.ListMyWorkspaces(ctx)
		if err != nil {
			return nil, fmt.Errorf("list dingtalk workspaces: %w", err)
		}
		resources := make([]types.Resource, 0, len(spaces))
		for _, s := range spaces {
			resources = append(resources, types.Resource{
				ExternalID:  s.WorkspaceID,
				Name:        s.Name,
				Type:        "wiki_workspace",
				Description: s.Description,
				URL:         s.URL,
				HasChildren: true,
				Metadata: map[string]interface{}{
					"workspace_id": s.WorkspaceID,
					"type":         s.Type,
				},
			})
		}
		return resources, nil
	}

	// Lazy load: list the direct children of the given workspace's root.
	ws, err := client.GetWorkspace(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("get workspace %s: %w", parentID, err)
	}
	nodes, err := client.ListNodes(ctx, ws.RootNodeID)
	if err != nil {
		return nil, fmt.Errorf("list nodes under %s: %w", parentID, err)
	}
	resources := make([]types.Resource, 0, len(nodes))
	for _, n := range nodes {
		resources = append(resources, nodeToResource(parentID, n))
	}
	return resources, nil
}

// ResolveResourceAncestors is a no-op for DingTalk MVP: ListResources returns a
// flat single-level list (workspace → root children), so there is no deep tree
// to reveal. Returning an empty slice is the documented behaviour for
// connectors that don't load a deep tree lazily.
func (c *Connector) ResolveResourceAncestors(
	ctx context.Context, config *types.DataSourceConfig, resourceIDs []string,
) ([]string, error) {
	return nil, nil
}

// FetchAll performs a full sync of all uploaded files under the selected
// workspaces (or specific nodes, if a resource ID is a node id).
func (c *Connector) FetchAll(
	ctx context.Context, config *types.DataSourceConfig, resourceIDs []string,
) ([]types.FetchedItem, error) {
	cfg, err := parseDingTalkConfig(config)
	if err != nil {
		return nil, err
	}
	client := NewClient(cfg)

	var allItems []types.FetchedItem
	for _, resourceID := range resourceIDs {
		nodes, err := c.listNodesForResource(ctx, client, resourceID)
		if err != nil {
			return nil, fmt.Errorf("list nodes for resource %s: %w", resourceID, err)
		}
		for _, node := range nodes {
			item, err := c.fetchNodeContent(ctx, client, node, resourceID)
			if err != nil {
				allItems = append(allItems, types.FetchedItem{
					ExternalID:       node.NodeID,
					Title:            node.Name,
					SourceResourceID: resourceID,
					Metadata:         map[string]string{"error": err.Error()},
				})
				continue
			}
			if item != nil {
				allItems = append(allItems, *item)
			}
		}
	}
	return allItems, nil
}

// FetchIncremental performs an incremental sync by comparing each node's
// modifiedTime against the previously recorded state. Mirrors Feishu's cursor
// strategy: traverse the full tree, diff by modifiedTime, detect deletions by
// set difference.
func (c *Connector) FetchIncremental(
	ctx context.Context, config *types.DataSourceConfig, cursor *types.SyncCursor,
) ([]types.FetchedItem, *types.SyncCursor, error) {
	cfg, err := parseDingTalkConfig(config)
	if err != nil {
		return nil, nil, err
	}
	client := NewClient(cfg)

	var prev dingtalkCursor
	if cursor != nil && cursor.ConnectorCursor != nil {
		b, _ := json.Marshal(cursor.ConnectorCursor)
		_ = json.Unmarshal(b, &prev)
	}

	newCursor := dingtalkCursor{
		LastSyncTime:      time.Now(),
		NodeModifiedTimes: make(map[string]map[string]string),
	}
	var changed []types.FetchedItem

	resourceIDs := config.ResourceIDs
	if len(resourceIDs) == 0 {
		return nil, nil, fmt.Errorf("no resource IDs configured")
	}

	for _, resourceID := range resourceIDs {
		nodes, err := c.listNodesForResource(ctx, client, resourceID)
		if err != nil {
			return nil, nil, fmt.Errorf("list nodes for resource %s: %w", resourceID, err)
		}

		newCursor.NodeModifiedTimes[resourceID] = make(map[string]string)
		currentNodes := make(map[string]bool)
		for _, node := range nodes {
			currentNodes[node.NodeID] = true
			mt := node.ModifiedTime
			newCursor.NodeModifiedTimes[resourceID][node.NodeID] = mt

			// Skip unchanged.
			if prev.NodeModifiedTimes != nil {
				if prevTimes, ok := prev.NodeModifiedTimes[resourceID]; ok {
					if prevMT, exists := prevTimes[node.NodeID]; exists && prevMT == mt {
						continue
					}
				}
			}

			item, err := c.fetchNodeContent(ctx, client, node, resourceID)
			if err != nil {
				changed = append(changed, types.FetchedItem{
					ExternalID:       node.NodeID,
					Title:            node.Name,
					SourceResourceID: resourceID,
					Metadata:         map[string]string{"error": err.Error()},
				})
				continue
			}
			if item != nil {
				changed = append(changed, *item)
			}
		}

		// Detect deletions: nodes in the previous cursor but not in the
		// current tree.
		if prev.NodeModifiedTimes != nil {
			if prevTimes, ok := prev.NodeModifiedTimes[resourceID]; ok {
				for nodeID := range prevTimes {
					if !currentNodes[nodeID] {
						changed = append(changed, types.FetchedItem{
							ExternalID:       nodeID,
							IsDeleted:        true,
							SourceResourceID: resourceID,
						})
					}
				}
			}
		}
	}

	nextCursorMap := make(map[string]interface{})
	b, _ := json.Marshal(newCursor)
	_ = json.Unmarshal(b, &nextCursorMap)
	nextSyncCursor := &types.SyncCursor{
		LastSyncTime:    time.Now(),
		ConnectorCursor: nextCursorMap,
	}
	return changed, nextSyncCursor, nil
}

// listNodesForResource resolves a resourceID — which may be either a
// workspaceId (user selected a whole knowledge base) or a nodeId (user
// selected a specific folder/document subtree) — to the full node tree to
// sync. It tries workspace first; if that fails it falls back to treating
// the id as a node. This is needed because ListResources returns nodeIds
// when the user expands a workspace and picks children.
func (c *Connector) listNodesForResource(ctx context.Context, client *Client, resourceID string) ([]wikiNode, error) {
	// Common case: a workspaceId selected at the top level.
	if nodes, err := client.ListNodesRecursiveFrom(ctx, resourceID, ""); err == nil {
		return nodes, nil
	}
	// Fallback: a nodeId picked from inside a workspace. GetNode returns the
	// node (with its WorkspaceID) and we walk its subtree.
	nodes, err := client.ListNodesRecursiveFrom(ctx, "", resourceID)
	if err != nil {
		return nil, fmt.Errorf("resource %q is neither a valid workspace nor a node: %w", resourceID, err)
	}
	return nodes, nil
}

// fetchNodeContent fetches the content of a single node and converts it to a
// FetchedItem. Dispatches by category:
//   - DOCUMENT/IMAGE/VIDEO/AUDIO/ARCHIVE (uploaded files) → download bytes
//   - ALIDOC (online docs .adoc/.axls/.able) → skip (async-only, phase 2)
//   - OTHER folders → no content, traversal only (nil item)
func (c *Connector) fetchNodeContent(
	ctx context.Context, client *Client, node wikiNode, resourceID string,
) (*types.FetchedItem, error) {
	if !isDownloadableCategory(node.Category) {
		if node.Category == "ALIDOC" {
			// ponytail: ALIDOC online docs need async Stream events; skip in MVP.
			logger.Warnf(ctx, "[DingTalk] skip ALIDOC online doc %s (%s): not supported in MVP",
				node.Name, node.Extension)
		}
		return nil, nil
	}

	data, err := client.DownloadFile(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", node.Name, err)
	}

	fileName := node.Name
	if fileName == "" {
		fileName = node.NodeID
	}
	// Ensure the file name carries its extension; DingTalk node.Name usually does.
	if node.Extension != "" && !strings.HasSuffix(strings.ToLower(fileName), "."+strings.ToLower(node.Extension)) {
		fileName = fileName + "." + node.Extension
	}

	return &types.FetchedItem{
		ExternalID:       node.NodeID,
		Title:            strings.TrimSuffix(node.Name, "."+node.Extension),
		Content:          data,
		ContentType:      "application/octet-stream",
		FileName:         sanitizeFileName(fileName),
		URL:              node.URL,
		UpdatedAt:        parseDingTalkTime(node.ModifiedTime),
		SourceResourceID: resourceID,
		Metadata: map[string]string{
			"node_id":      node.NodeID,
			"workspace_id": node.WorkspaceID,
			"category":     node.Category,
			"extension":    node.Extension,
			"creator_id":   node.CreatorID,
			"channel":      types.ChannelDingtalk,
		},
	}, nil
}

// --- helpers ---

// isDownloadableCategory returns true for node categories that are uploaded
// binary files (downloadable via storage API).
func isDownloadableCategory(category string) bool {
	switch category {
	case "DOCUMENT", "IMAGE", "VIDEO", "AUDIO", "ARCHIVE":
		return true
	default:
		return false
	}
}

func nodeToResource(workspaceID string, n wikiNode) types.Resource {
	name := n.Name
	if name == "" {
		name = n.NodeID
	}
	return types.Resource{
		ExternalID:  n.NodeID,
		Name:        name,
		Type:        "wiki_node",
		URL:         n.URL,
		ParentID:    workspaceID,
		HasChildren: n.HasChildren,
		ModifiedAt:  parseDingTalkTime(n.ModifiedTime),
		Metadata: map[string]interface{}{
			"workspace_id": workspaceID,
			"node_id":      n.NodeID,
			"category":     n.Category,
			"extension":    n.Extension,
		},
	}
}

// parseDingTalkConfig extracts and validates DingTalk-specific configuration.
func parseDingTalkConfig(config *types.DataSourceConfig) (*Config, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}
	credBytes, err := json.Marshal(config.Credentials)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(credBytes, &cfg); err != nil {
		return nil, fmt.Errorf("parse dingtalk credentials: %w", err)
	}
	if cfg.AppKey == "" || cfg.AppSecret == "" {
		return nil, fmt.Errorf("dingtalk app_key and app_secret are required")
	}
	if cfg.UserID == "" {
		return nil, fmt.Errorf("dingtalk user_id is required (operator identity for document APIs)")
	}
	return &cfg, nil
}

// parseDingTalkTime parses a DingTalk RFC3339-ish timestamp (e.g.
// "2026-06-24T15:13Z"). Returns zero time on empty/invalid input.
func parseDingTalkTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// DingTalk uses "2006-01-02T15:04Z07:00" (RFC3339 without fractional
	// seconds). time.Parse handles the trailing "Z".
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// sanitizeFileName removes characters that are invalid in filenames and
// truncates at a UTF-8 rune boundary. Mirrors the Feishu connector helper.
func sanitizeFileName(name string) string {
	if name == "" {
		return "untitled"
	}
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	result := replacer.Replace(name)
	const maxBytes = 200
	if len(result) > maxBytes {
		result = result[:maxBytes]
		// Trim back to a UTF-8 boundary.
		for len(result) > 0 && !utf8RuneStart(result[len(result)-1]) {
			result = result[:len(result)-1]
		}
	}
	return result
}

// utf8RuneStart reports whether b is the first byte of a UTF-8 rune. Used by
// sanitizeFileName to avoid splitting a multi-byte codepoint.
func utf8RuneStart(b byte) bool {
	return b&0xC0 != 0x80
}

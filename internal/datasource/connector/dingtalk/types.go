// Package dingtalk implements the DingTalk (钉钉) data source connector for WeKnora.
//
// It syncs uploaded documents (PDF/Word/Excel/...) from DingTalk knowledge bases
// (wiki spaces) into WeKnora knowledge bases. DingTalk online documents (.adoc
// in-line docs, .axls sheets, .able multidimensional tables) are NOT supported
// in this version: their content can only be obtained asynchronously via the
// DingTalk Stream event bus, which does not fit the Connector request/response
// model. See package docs for the rationale.
//
// DingTalk API docs (verified endpoints):
//   - Auth (access_token):  https://open.dingtalk.com/document/orgapp/obtain-the-access_token-of-an-internal-app
//   - user → unionId:       https://open.dingtalk.com/document/orgapp/query-details-about-a-user-by-userid
//   - Get node:             GET  /v2.0/wiki/nodes/{nodeId}
//   - List child nodes:     GET  /v2.0/wiki/nodes?parentNodeId=...
//   - Get workspace:        GET  /v2.0/wiki/workspaces/{workspaceId}
//   - My workspaces:        GET  /v2.0/wiki/mineWorkspaces
//   - node → dentry id:     GET  /v2.0/doc/dentries/{nodeId}/queryDentryId
//   - Download info:        POST /v1.0/storage/spaces/{spaceId}/dentries/{dentryId}/downloadInfos/query
package dingtalk

import "time"

// Config holds DingTalk-specific configuration for the data source connector.
//
// DingTalk uses the enterprise self-built app (企业自建应用) model, same as
// Feishu. Unlike Feishu, DingTalk document APIs require an operator identity
// (unionId) for every call; the unionId is resolved from UserID via the contact
// API, so UserID is mandatory.
type Config struct {
	// AppKey from DingTalk developer console (ding...).
	AppKey string `json:"app_key"`

	// AppSecret from DingTalk developer console.
	AppSecret string `json:"app_secret"`

	// UserID of the operator. Looks like "BL06242" or a numeric id.
	// Resolved to unionId at runtime; unionId is used as operatorId for all
	// wiki/doc/storage API calls.
	UserID string `json:"user_id"`

	// BaseURL for DingTalk API (default: https://api.dingtalk.com).
	// Kept for symmetry with the Feishu connector; DingTalk has no
	// international variant that changes this host.
	BaseURL string `json:"base_url,omitempty"`
}

// DefaultBaseURL is the default DingTalk Open Platform API base URL.
const DefaultBaseURL = "https://api.dingtalk.com"

// GetBaseURL returns the effective base URL, defaulting to DingTalk if not set.
func (c *Config) GetBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

// --- DingTalk API response structures ---
//
// DingTalk's new gateway (api.dingtalk.com) returns errors as:
//   {"code":"...","message":"...","requestid":"..."}
// Success responses carry the payload directly (no wrapper code), e.g. GetNode
// returns {"node":{...}}. We model only the fields we use.

// apiError is the common error shape returned by the DingTalk gateway.
type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestid"`
}

// tokenResponse is the response for POST /v1.0/oauth2/accessToken.
type tokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpireIn    int    `json:"expireIn"` // seconds
	// Error fields (populated on failure)
	Code    string `json:"code"`
	Message string `json:"message"`
}

// getNodeResponse is the response for GET /v2.0/wiki/nodes/{nodeId}.
type getNodeResponse struct {
	Node wikiNode `json:"node"`
	apiError
}

// listNodesResponse is the response for GET /v2.0/wiki/nodes.
type listNodesResponse struct {
	Nodes []wikiNode `json:"nodes"`
	// nextToken is non-null when more pages exist; absent/null when done.
	NextToken string `json:"nextToken"`
	apiError
}

// wikiNode represents a node (document or folder) in a DingTalk knowledge base.
//
// Category discriminates the retrieval strategy:
//   - DOCUMENT: an uploaded binary file (PDF/Word/Excel/...). Downloadable.
//   - ALIDOC:   a DingTalk online document (.adoc/.axls/.able). Async-only.
//   - OTHER:    a folder (hasChildren=true, no content). Traversal only.
//   - IMAGE/VIDEO/AUDIO/ARCHIVE: also uploaded files, treated like DOCUMENT.
type wikiNode struct {
	NodeID       string `json:"nodeId"`
	WorkspaceID  string `json:"workspaceId"`
	Name         string `json:"name"`
	Type         string `json:"type"`      // "FILE" or "FOLDER"
	Category     string `json:"category"`  // DOCUMENT | ALIDOC | OTHER | IMAGE | ...
	Extension    string `json:"extension"` // "pdf", "docx", "xlsx", "adoc", ...
	Size         int64  `json:"size"`
	HasChildren  bool   `json:"hasChildren"`
	URL          string `json:"url"`
	CreateTime   string `json:"createTime"`   // RFC3339-ish, e.g. "2026-06-24T15:13Z"
	ModifiedTime string `json:"modifiedTime"` // same format
	CreatorID    string `json:"creatorId"`
	ModifierID   string `json:"modifierId"`
}

// getWorkspaceResponse is the response for GET /v2.0/wiki/workspaces/{workspaceId}.
type getWorkspaceResponse struct {
	Workspace wikiSpace `json:"workspace"`
	apiError
}

// mineWorkspacesResponse is the response for GET /v2.0/wiki/mineWorkspaces.
// Returns the operator's personal workspace (singular "workspace" key).
type mineWorkspacesResponse struct {
	Workspace wikiSpace `json:"workspace"`
	apiError
}

// wikiSpace represents a DingTalk knowledge base (workspace).
type wikiSpace struct {
	WorkspaceID string `json:"workspaceId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`       // PERSONAL | TEAM
	RootNodeID  string `json:"rootNodeId"` // virtual root; traverse from here
	URL         string `json:"url"`
	CreatorID   string `json:"creatorId"`
}

// queryDentryIDResponse is the response for GET /v2.0/doc/dentries/{nodeId}/queryDentryId.
// Translates a wiki node id to the storage dentry id needed for download.
type queryDentryIDResponse struct {
	DentryID   string `json:"dentryId"`
	SpaceID    string `json:"spaceId"`
	DentryUUID string `json:"dentryUuid"`
	apiError
}

// queryDocContentResponse is the response for GET /v2.0/doc/query/{nodeId}/contents.
// DingTalk's doc-content APIs are asynchronous: they return a taskId and the
// actual markdown body is delivered later via the Stream doc_content_export_result
// event. The taskId is logged for correlation but matching is done by DocURL.
type queryDocContentResponse struct {
	TaskID int64 `json:"taskId"`
	apiError
}

// downloadInfoResponse is the response for POST
// /v1.0/storage/spaces/{spaceId}/dentries/{dentryId}/downloadInfos/query.
// Returns a short-lived (900s) signed URL plus the headers required to GET it.
type downloadInfoResponse struct {
	Protocol            string `json:"protocol"` // "HEADER_SIGNATURE"
	HeaderSignatureInfo struct {
		Headers           map[string]string `json:"headers"`
		ResourceURLs      []string          `json:"resourceUrls"`
		ExpirationSeconds int               `json:"expirationSeconds"`
	} `json:"headerSignatureInfo"`
	apiError
}

// userGetResult is the relevant subset of oapi /topapi/v2/user/get result.
type userGetResult struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	Result  struct {
		UnionID string `json:"unionid"`
		Name    string `json:"name"`
	} `json:"result"`
}

// dingtalkCursor stores incremental sync state for DingTalk.
//
// Mirrors the Feishu cursor: per-resource map of nodeID → last seen
// modifiedTime. Incremental sync compares modifiedTime to detect changes and
// uses the node set difference to detect deletions.
type dingtalkCursor struct {
	// LastSyncTime is the timestamp of the last successful sync.
	LastSyncTime time.Time `json:"last_sync_time"`

	// NodeModifiedTimes maps resourceID → nodeID → last known modifiedTime.
	NodeModifiedTimes map[string]map[string]string `json:"node_modified_times,omitempty"`
}

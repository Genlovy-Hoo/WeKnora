package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
)

// Client wraps the DingTalk Open Platform API for knowledge-base document
// operations. It is the DingTalk counterpart of feishu.Client.
type Client struct {
	baseURL   string
	appKey    string
	appSecret string
	userID    string

	httpClient *http.Client

	// access_token cache (thread-safe). DingTalk tokens expire in 7200s;
	// we refresh 5 minutes early, matching the Feishu connector.
	tokenMu    sync.Mutex
	tokenCache string
	tokenExpAt time.Time

	// unionId cache (thread-safe). Resolved once from userID via the contact
	// API; every wiki/doc/storage call uses it as operatorId.
	unionMu    sync.Mutex
	unionCache string
}

// NewClient creates a new DingTalk API client.
func NewClient(config *Config) *Client {
	return &Client{
		baseURL:    config.GetBaseURL(),
		appKey:     config.AppKey,
		appSecret:  config.AppSecret,
		userID:     config.UserID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// getAccessToken retrieves (or returns cached) the application access_token.
// Endpoint: POST /v1.0/oauth2/accessToken
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.tokenCache != "" && time.Now().Before(c.tokenExpAt) {
		return c.tokenCache, nil
	}

	payload, _ := json.Marshal(map[string]string{
		"appKey":    c.appKey,
		"appSecret": c.appSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1.0/oauth2/accessToken", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result tokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("dingtalk auth error: code=%s msg=%s", result.Code, result.Message)
	}

	c.tokenCache = result.AccessToken
	ttl := time.Duration(result.ExpireIn) * time.Second
	if ttl > 5*time.Minute {
		ttl -= 5 * time.Minute
	}
	c.tokenExpAt = time.Now().Add(ttl)

	logger.Infof(ctx, "[DingTalk] got access_token: %s... expire=%ds",
		truncate(result.AccessToken, 8), result.ExpireIn)
	return c.tokenCache, nil
}

// getUnionID resolves the operator unionId from UserID.
// Endpoint: POST oapi.dingtalk.com/topapi/v2/user/get
//
// Cached for the client's lifetime; unionId is stable per user.
func (c *Client) getUnionID(ctx context.Context) (string, error) {
	c.unionMu.Lock()
	defer c.unionMu.Unlock()

	if c.unionCache != "" {
		return c.unionCache, nil
	}
	if c.userID == "" {
		return "", fmt.Errorf("dingtalk user_id is required (needed to resolve operator unionId)")
	}

	payload, _ := json.Marshal(map[string]string{"userid": c.userID})
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return "", err
	}
	// DingTalk's contact API lives on a different host (oapi.dingtalk.com)
	// than the new gateway (api.dingtalk.com). For testability we route it
	// through baseURL when it is non-default; in production baseURL is the
	// default api host and we fall back to the real oapi host.
	oapiHost := "https://oapi.dingtalk.com"
	if c.baseURL != DefaultBaseURL {
		oapiHost = c.baseURL // test mock
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		oapiHost+"/topapi/v2/user/get?access_token="+url.QueryEscape(token),
		bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create user request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request user: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result userGetResult
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode user response: %w", err)
	}
	if result.ErrCode != 0 || result.Result.UnionID == "" {
		return "", fmt.Errorf("dingtalk get user unionId failed: errcode=%d errmsg=%s (userid %q may be outside the app's contact scope)",
			result.ErrCode, result.ErrMsg, c.userID)
	}

	c.unionCache = result.Result.UnionID
	logger.Infof(ctx, "[DingTalk] resolved unionId for userid=%s name=%s", c.userID, result.Result.Name)
	return c.unionCache, nil
}

// doRequest executes an authenticated DingTalk API request (JSON in/out).
// operatorId is added to the query when includeOperator is true.
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, body interface{}, result interface{}) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}
	unionID, err := c.getUnionID(ctx)
	if err != nil {
		return err
	}

	// Every wiki/doc API needs operatorId; merge it into the query.
	if query == nil {
		query = url.Values{}
	}
	if query.Get("operatorId") == "" {
		query.Set("operatorId", unionID)
	}

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	logger.Infof(ctx, "[DingTalk] %s %s", method, path)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	logger.Infof(ctx, "[DingTalk] %s %s → status=%d bodyLen=%d body=%s",
		method, path, resp.StatusCode, len(respBody), truncate(string(respBody), 1000))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dingtalk api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Ping verifies credentials by fetching access_token + unionId.
func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.getAccessToken(ctx); err != nil {
		return err
	}
	if _, err := c.getUnionID(ctx); err != nil {
		return err
	}
	return nil
}

// ListMyWorkspaces returns the operator's personal workspace.
// Endpoint: GET /v2.0/wiki/mineWorkspaces
//
// DingTalk's "list all workspaces" API requires a name filter and is awkward
// to use for discovery; mineWorkspaces reliably returns the personal space,
// which is the common case. Team spaces can be added by workspaceId later.
func (c *Client) ListMyWorkspaces(ctx context.Context) ([]wikiSpace, error) {
	var resp mineWorkspacesResponse
	if err := c.doRequest(ctx, http.MethodGet, "/v2.0/wiki/mineWorkspaces", nil, nil, &resp); err != nil {
		return nil, fmt.Errorf("list my workspaces: %w", err)
	}
	if resp.Code != "" {
		return nil, fmt.Errorf("list my workspaces error: code=%s msg=%s", resp.Code, resp.Message)
	}
	if resp.Workspace.WorkspaceID == "" {
		return nil, nil
	}
	return []wikiSpace{resp.Workspace}, nil
}

// GetWorkspace returns metadata for a workspace, including its RootNodeID
// (the virtual root from which node traversal starts).
// Endpoint: GET /v2.0/wiki/workspaces/{workspaceId}
func (c *Client) GetWorkspace(ctx context.Context, workspaceID string) (wikiSpace, error) {
	var resp getWorkspaceResponse
	if err := c.doRequest(ctx, http.MethodGet,
		"/v2.0/wiki/workspaces/"+workspaceID, nil, nil, &resp); err != nil {
		return wikiSpace{}, fmt.Errorf("get workspace: %w", err)
	}
	if resp.Code != "" {
		return wikiSpace{}, fmt.Errorf("get workspace error: code=%s msg=%s", resp.Code, resp.Message)
	}
	return resp.Workspace, nil
}

// GetNode returns metadata for a single node.
// Endpoint: GET /v2.0/wiki/nodes/{nodeId}
func (c *Client) GetNode(ctx context.Context, nodeID string) (wikiNode, error) {
	var resp getNodeResponse
	if err := c.doRequest(ctx, http.MethodGet,
		"/v2.0/wiki/nodes/"+nodeID, nil, nil, &resp); err != nil {
		return wikiNode{}, fmt.Errorf("get node: %w", err)
	}
	if resp.Code != "" {
		return wikiNode{}, fmt.Errorf("get node error: code=%s msg=%s", resp.Code, resp.Message)
	}
	return resp.Node, nil
}

// ListNodes lists the direct children of a parent node.
// Endpoint: GET /v2.0/wiki/nodes?parentNodeId=...&maxResults=50&nextToken=...
//
// If parentID is empty, callers should pass the workspace's RootNodeID.
func (c *Client) ListNodes(ctx context.Context, parentID string) ([]wikiNode, error) {
	var all []wikiNode
	nextToken := ""
	for {
		q := url.Values{}
		q.Set("parentNodeId", parentID)
		q.Set("maxResults", "50")
		if nextToken != "" {
			q.Set("nextToken", nextToken)
		}
		var resp listNodesResponse
		if err := c.doRequest(ctx, http.MethodGet, "/v2.0/wiki/nodes", q, nil, &resp); err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		if resp.Code != "" {
			return nil, fmt.Errorf("list nodes error: code=%s msg=%s", resp.Code, resp.Message)
		}
		all = append(all, resp.Nodes...)
		if resp.NextToken == "" {
			break
		}
		nextToken = resp.NextToken
	}
	return all, nil
}

// ListNodesRecursiveFrom returns the node itself (if nodeID != "") plus all
// descendants. If nodeID is empty, returns the whole workspace tree starting
// from the workspace root.
func (c *Client) ListNodesRecursiveFrom(ctx context.Context, workspaceID, nodeID string) ([]wikiNode, error) {
	// Resolve the traversal root.
	var root wikiNode
	var err error
	if nodeID == "" {
		ws, err := c.GetWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		if ws.RootNodeID == "" {
			return nil, fmt.Errorf("workspace %s has no root node id", workspaceID)
		}
		// The workspace root is virtual; start by listing its children.
		children, err := c.ListNodes(ctx, ws.RootNodeID)
		if err != nil {
			return nil, err
		}
		var all []wikiNode
		walk(ctx, c, children, &all)
		return all, nil
	}
	root, err = c.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	all := []wikiNode{root}
	if root.HasChildren {
		children, err := c.ListNodes(ctx, root.NodeID)
		if err != nil {
			return append(all, children...), err
		}
		walk(ctx, c, children, &all)
	}
	return all, nil
}

// walk depth-first appends every descendant to *out.
func walk(ctx context.Context, c *Client, nodes []wikiNode, out *[]wikiNode) {
	for _, n := range nodes {
		*out = append(*out, n)
		if n.HasChildren {
			children, err := c.ListNodes(ctx, n.NodeID)
			if err != nil {
				logger.Warnf(ctx, "[DingTalk] list children of %s: %v", n.NodeID, err)
				continue
			}
			walk(ctx, c, children, out)
		}
	}
}

// QueryDentryID translates a wiki node id to a storage dentry id + space id.
// Required before downloading an uploaded file.
// Endpoint: GET /v2.0/doc/dentries/{nodeId}/queryDentryId
func (c *Client) QueryDentryID(ctx context.Context, nodeID string) (dentryID, spaceID string, err error) {
	var resp queryDentryIDResponse
	if err := c.doRequest(ctx, http.MethodGet,
		"/v2.0/doc/dentries/"+nodeID+"/queryDentryId", nil, nil, &resp); err != nil {
		return "", "", fmt.Errorf("query dentry id: %w", err)
	}
	if resp.Code != "" {
		return "", "", fmt.Errorf("query dentry id error: code=%s msg=%s", resp.Code, resp.Message)
	}
	return resp.DentryID, resp.SpaceID, nil
}

// DownloadFile downloads an uploaded file node and returns its bytes.
// Flow: node → dentryId/spaceId → signed download URL → GET bytes.
// Only works for category=DOCUMENT (and IMAGE/VIDEO/...) uploaded files;
// ALIDOC online docs have no binary to download.
func (c *Client) DownloadFile(ctx context.Context, node wikiNode) ([]byte, error) {
	dentryID, spaceID, err := c.QueryDentryID(ctx, node.NodeID)
	if err != nil {
		return nil, err
	}

	// Fetch the short-lived signed download URL + required headers.
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	dlURL := fmt.Sprintf("%s/v1.0/storage/spaces/%s/dentries/%s/downloadInfos/query",
		c.baseURL, spaceID, dentryID)
	dlBody, _ := json.Marshal(map[string]interface{}{})
	dlReq, err := http.NewRequestWithContext(ctx, http.MethodPost, dlURL+"?unionId="+url.QueryEscape(c.unionCache), bytes.NewReader(dlBody))
	if err != nil {
		return nil, fmt.Errorf("create download-info request: %w", err)
	}
	dlReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	dlReq.Header.Set("x-acs-dingtalk-access-token", token)

	dlResp, err := c.httpClient.Do(dlReq)
	if err != nil {
		return nil, fmt.Errorf("request download info: %w", err)
	}
	defer dlResp.Body.Close()
	dlRespBody, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read download-info body: %w", err)
	}
	if dlResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download info failed: status=%d body=%s", dlResp.StatusCode, string(dlRespBody))
	}
	var info downloadInfoResponse
	if err := json.Unmarshal(dlRespBody, &info); err != nil {
		return nil, fmt.Errorf("decode download info: %w", err)
	}
	if info.Code != "" {
		return nil, fmt.Errorf("download info error: code=%s msg=%s", info.Code, info.Message)
	}
	if len(info.HeaderSignatureInfo.ResourceURLs) == 0 {
		return nil, fmt.Errorf("download info returned no resource urls")
	}

	// GET the actual file bytes with the signed headers.
	fileURL := info.HeaderSignatureInfo.ResourceURLs[0]
	fileReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create file download request: %w", err)
	}
	for k, v := range info.HeaderSignatureInfo.Headers {
		fileReq.Header.Set(k, v)
	}

	fileResp, err := c.httpClient.Do(fileReq)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}
	defer fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(fileResp.Body)
		return nil, fmt.Errorf("download file failed: status=%d body=%s", fileResp.StatusCode, truncate(string(b), 500))
	}
	data, err := io.ReadAll(fileResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file body: %w", err)
	}
	logger.Infof(ctx, "[DingTalk] downloaded %s → %d bytes", node.Name, len(data))
	return data, nil
}

// truncate truncates a string to maxLen and appends "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

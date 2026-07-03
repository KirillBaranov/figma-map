package figma

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// apiV1 is the prefix for every versioned data-plane endpoint the backend
// exposes (ADR-0003 §2). /ping stays unprefixed — it's infrastructure, not
// the data API.
const apiV1 = "/api/v1"

// Bridge is a Source backed by a running figma-map backend's HTTP RPC server.
// It speaks the backend's POST /api/v1/rpc protocol and maps responses into
// the domain Node model. No Figma REST API limits apply.
type Bridge struct {
	baseURL string
	client  *http.Client
}

// NewBridge returns a Bridge talking to the given base URL (e.g.
// http://localhost:1994).
func NewBridge(baseURL string) *Bridge {
	return &Bridge{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 40 * time.Second},
	}
}

// rpcRequest is the wire shape accepted by the bridge's /rpc endpoint.
type rpcRequest struct {
	Tool    string         `json:"tool"`
	NodeIDs []string       `json:"nodeIds,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
	FileKey string         `json:"fileKey,omitempty"`
}

// rpcResponse is the wire shape returned by the bridge. Data is left raw so
// each caller can decode the tool-specific payload.
type rpcResponse struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

// Ping reports whether the bridge is reachable and healthy.
func (b *Bridge) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+"/ping", nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge /ping returned %d", resp.StatusCode)
	}
	return nil
}

// rpc performs a single tool call and returns the raw data payload.
func (b *Bridge) rpc(ctx context.Context, req rpcRequest) (json.RawMessage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+apiV1+"/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bridge unreachable at %s: %w", b.baseURL, err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bridge %s returned %d: %s", req.Tool, httpResp.StatusCode, string(raw))
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(raw, &rpcResp); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", req.Tool, err)
	}
	if rpcResp.Error != "" {
		return nil, fmt.Errorf("bridge %s: %s", req.Tool, rpcResp.Error)
	}
	return rpcResp.Data, nil
}

// Files implements Source.
func (b *Bridge) Files(ctx context.Context) ([]File, error) {
	data, err := b.rpc(ctx, rpcRequest{Tool: "list_files"})
	if err != nil {
		return nil, err
	}
	var files []File
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("decode files: %w", err)
	}
	return files, nil
}

// Document implements Source.
func (b *Bridge) Document(ctx context.Context, fileKey string) (*Node, error) {
	return b.DocumentWithDepth(ctx, fileKey, 0)
}

// DocumentWithDepth implements Source.
func (b *Bridge) DocumentWithDepth(ctx context.Context, fileKey string, depth int) (*Node, error) {
	req := rpcRequest{Tool: "get_document", FileKey: fileKey}
	if depth > 0 {
		req.Params = map[string]any{"depth": depth}
	}
	data, err := b.rpc(ctx, req)
	if err != nil {
		return nil, err
	}
	var node Node
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}
	return &node, nil
}

// Node implements Source. The bridge returns get_node payloads as either a
// single object or a one-element array depending on version, so both are
// accepted.
func (b *Bridge) Node(ctx context.Context, fileKey, id string) (*Node, error) {
	return b.NodeWithDepth(ctx, fileKey, id, 0)
}

// NodeWithDepth implements Source.
func (b *Bridge) NodeWithDepth(ctx context.Context, fileKey, id string, depth int) (*Node, error) {
	req := rpcRequest{Tool: "get_node", NodeIDs: []string{id}, FileKey: fileKey}
	if depth > 0 {
		req.Params = map[string]any{"depth": depth}
	}
	data, err := b.rpc(ctx, req)
	if err != nil {
		return nil, err
	}
	return decodeSingleNode(data)
}

// Selection implements Source. Fetched at depth 1: service.SelectionNode
// only surfaces the selected node's own fields (id/name/type/text/bounds/
// variantModes/componentProps), never its descendants, so there's no reason
// to pay for fully serializing — styles, bound variables, dev-resources —
// the rest of a potentially huge selected subtree.
func (b *Bridge) Selection(ctx context.Context, fileKey string) ([]Node, error) {
	data, err := b.rpc(ctx, rpcRequest{
		Tool:    "get_selection",
		FileKey: fileKey,
		Params:  map[string]any{"depth": 1},
	})
	if err != nil {
		return nil, err
	}
	var nodes []Node
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("decode selection: %w", err)
	}
	return nodes, nil
}

// screenshotResponse mirrors the get_screenshot data payload.
type screenshotResponse struct {
	Exports []struct {
		NodeID   string  `json:"nodeId"`
		NodeName string  `json:"nodeName"`
		Base64   string  `json:"base64"`
		Width    float64 `json:"width"`
		Height   float64 `json:"height"`
		// Optional crop applied when the node had no background and the plugin
		// exported a background-providing ancestor instead.
		CropX *int `json:"cropX,omitempty"`
		CropY *int `json:"cropY,omitempty"`
		CropW *int `json:"cropW,omitempty"`
		CropH *int `json:"cropH,omitempty"`
	} `json:"exports"`
}

// Screenshot implements Source.
func (b *Bridge) Screenshot(ctx context.Context, fileKey, id string, opts ScreenshotOpts) ([]byte, error) {
	params := map[string]any{}
	if opts.Format != "" {
		params["format"] = opts.Format
	} else {
		params["format"] = "PNG"
	}
	if opts.Scale > 0 {
		params["scale"] = opts.Scale
	}

	data, err := b.rpc(ctx, rpcRequest{
		Tool:    "get_screenshot",
		NodeIDs: []string{id},
		Params:  params,
		FileKey: fileKey,
	})
	if err != nil {
		return nil, err
	}

	var resp screenshotResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}
	if len(resp.Exports) == 0 {
		return nil, fmt.Errorf("no screenshot returned for node %s", id)
	}
	e := resp.Exports[0]
	pngBytes, err := base64.StdEncoding.DecodeString(e.Base64)
	if err != nil {
		return nil, err
	}
	if e.CropX != nil {
		pngBytes, err = cropPNG(pngBytes, *e.CropX, *e.CropY, *e.CropW, *e.CropH)
		if err != nil {
			return nil, fmt.Errorf("crop screenshot: %w", err)
		}
	}
	return pngBytes, nil
}

// Metadata implements Source.
func (b *Bridge) Metadata(ctx context.Context, fileKey string) (Metadata, error) {
	data, err := b.rpc(ctx, rpcRequest{Tool: "get_metadata", FileKey: fileKey})
	if err != nil {
		return Metadata{}, err
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	return meta, nil
}

// VariableDefs implements Source.
func (b *Bridge) VariableDefs(ctx context.Context, fileKey string) (VariableDefs, error) {
	data, err := b.rpc(ctx, rpcRequest{Tool: "get_variable_defs", FileKey: fileKey})
	if err != nil {
		return VariableDefs{}, err
	}
	var defs VariableDefs
	if err := json.Unmarshal(data, &defs); err != nil {
		return VariableDefs{}, fmt.Errorf("decode variable defs: %w", err)
	}
	return defs, nil
}

// FindNodes implements Source.
func (b *Bridge) FindNodes(ctx context.Context, fileKey string, opts FindNodesOptions) ([]FindMatch, error) {
	params := map[string]any{}
	if opts.Query != "" {
		params["query"] = opts.Query
	}
	if opts.TextQuery != "" {
		params["textQuery"] = opts.TextQuery
	}
	if opts.NodeType != "" {
		params["nodeType"] = opts.NodeType
	}
	if opts.Mode != "" {
		params["mode"] = opts.Mode
	}
	if opts.WithinNodeID != "" {
		params["withinNodeId"] = opts.WithinNodeID
	}
	if opts.MaxDepth > 0 {
		params["maxDepth"] = opts.MaxDepth
	}
	if opts.MaxResults > 0 {
		params["maxResults"] = opts.MaxResults
	}

	data, err := b.rpc(ctx, rpcRequest{Tool: "find_nodes", FileKey: fileKey, Params: params})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Matches []FindMatch `json:"matches"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode find_nodes: %w", err)
	}
	return resp.Matches, nil
}

// MainComponentName implements Source.
func (b *Bridge) MainComponentName(ctx context.Context, fileKey, id string) (string, error) {
	data, err := b.rpc(ctx, rpcRequest{Tool: "get_main_component_name", NodeIDs: []string{id}, FileKey: fileKey})
	if err != nil {
		return "", err
	}
	var resp struct {
		Name *string `json:"name"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("decode get_main_component_name: %w", err)
	}
	if resp.Name == nil {
		return "", nil
	}
	return *resp.Name, nil
}

// decodeSingleNode handles both the object and one-element-array shapes the
// bridge may return for get_node.
func decodeSingleNode(data json.RawMessage) (*Node, error) {
	var node Node
	if err := json.Unmarshal(data, &node); err == nil && node.ID != "" {
		return &node, nil
	}
	var nodes []Node
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("decode node: %w", err)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("node not found")
	}
	return &nodes[0], nil
}

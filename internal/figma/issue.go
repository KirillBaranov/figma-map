package figma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// IssueBBox is the pixel rectangle on the page a flagged issue refers to.
type IssueBBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// IssueDiffRegion is one cell of a pixel-diff grid, precomputed by whoever
// flagged the issue (optional — figma-map can also compute this itself via
// pixeldiff-images).
type IssueDiffRegion struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
	DiffPct float64 `json:"diffPct"`
}

// Issue is a region of a live page flagged by a human via the browser
// extension, for the agent to pick up and act on. It carries ground truth
// only (screenshot, bounds, selector) — no diffing or matching happens on
// the client; that stays in figma-map per ADR-0001.
type Issue struct {
	ID          string `json:"id"`
	TabURL      string `json:"tabUrl"`
	Selector    string `json:"selector"`
	FigmaNodeID string `json:"figmaNodeId,omitempty"`
	// RegionNodeID is the specific descendant of FigmaNodeID the human
	// pinpointed via the overlay's click-to-node hit-map, when finer-grained
	// than the root node that was screenshotted. Empty when the issue was
	// flagged without a live Figma selection (e.g. manual paste mode).
	RegionNodeID     string            `json:"regionNodeId,omitempty"`
	RegionBounds     *IssueBBox        `json:"regionBounds,omitempty"`
	FileKey          string            `json:"fileKey,omitempty"`
	Bounds           IssueBBox         `json:"bbox"`
	ScreenshotBase64 string            `json:"screenshotBase64"`
	DiffPct          *float64          `json:"diffPct,omitempty"`
	DiffRegions      []IssueDiffRegion `json:"diffRegions,omitempty"`
	Note             string            `json:"note,omitempty"`
	CreatedAt        string            `json:"createdAt"`
}

// IssueSource is implemented by Bridge to list/ack issues flagged by the
// browser extension. Kept separate from Source: issues are page captures
// relayed by the bridge process, not Figma data, so requiring every Source
// implementation (fakes included) to handle them would be the wrong coupling.
type IssueSource interface {
	ListIssues(ctx context.Context, fileKey string) ([]Issue, error)
	AckIssue(ctx context.Context, id string) error
}

// ListIssues fetches pending issues from the bridge's /issues inbox,
// optionally filtered to a single file.
func (b *Bridge) ListIssues(ctx context.Context, fileKey string) ([]Issue, error) {
	url := b.baseURL + apiV1 + "/issues"
	if fileKey != "" {
		url += "?fileKey=" + fileKey
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bridge unreachable at %s: %w", b.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bridge /issues returned %d: %s", resp.StatusCode, string(raw))
	}

	var out rpcResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode /issues response: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("bridge /issues: %s", out.Error)
	}
	var issues []Issue
	if len(out.Data) > 0 {
		if err := json.Unmarshal(out.Data, &issues); err != nil {
			return nil, fmt.Errorf("decode issues: %w", err)
		}
	}
	return issues, nil
}

// AckIssue marks a flagged issue as handled, removing it from the inbox.
func (b *Bridge) AckIssue(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, b.baseURL+apiV1+"/issues/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("bridge unreachable at %s: %w", b.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("issue %s not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bridge /issues/%s returned %d: %s", id, resp.StatusCode, string(raw))
	}
	return nil
}

package figma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// RESTSource is a Source backed directly by the Figma REST API
// (api.figma.com), for headless/CI/server-side agents that can't run an open
// Figma desktop session with the bridge plugin (ADR-0003 §5). It is strictly
// additive and read-only: figmaFileKey is fixed at construction (REST has no
// notion of "currently open files" the way the live bridge does), and it
// does not implement IssueSource — a REST snapshot has no live DOM to
// capture issues from, the same reasoning ADR-0002 already applies to keep
// IssueSource separate from Source.
//
// Scope: only what a static file snapshot can answer. Editor-only concepts
// (the current selection, which page is open) have no REST equivalent and
// return a clear error rather than a guess. Some Node enrichment fields the
// bridge fills from a live document (bound-variable resolution beyond
// fills/strokes, prototyping Reactions, DevResources, Annotations,
// GridPosition) are not mapped here — left at their Go zero value — since
// they would require several more REST calls per node for comparatively
// rare consumers; this can be extended later without changing the
// interface.
type RESTSource struct {
	token   string
	fileKey string
	client  *http.Client
	baseURL string // overridable in tests; defaults to https://api.figma.com
}

// NewRESTSource returns a Source for the given file, authenticated with a
// Figma personal access token or Dev Mode token (see ADR-0003 §5 —
// Dev Mode/Enterprise-plan-gated for some endpoints, e.g. VariableDefs).
func NewRESTSource(token, fileKey string) *RESTSource {
	return &RESTSource{
		token:   token,
		fileKey: fileKey,
		client:  &http.Client{Timeout: 40 * time.Second},
		baseURL: "https://api.figma.com",
	}
}

func (r *RESTSource) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := r.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Figma-Token", r.token)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("figma REST API unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("figma REST API %s returned %d: %s", path, resp.StatusCode, string(raw))
	}
	return raw, nil
}

// Ping implements Source.
func (r *RESTSource) Ping(ctx context.Context) error {
	if r.fileKey == "" {
		return fmt.Errorf("REST source has no fileKey configured (figma-map.yaml: fileKey)")
	}
	_, err := r.get(ctx, "/v1/files/"+r.fileKey, url.Values{"depth": {"1"}})
	return err
}

// Files implements Source. REST has no concept of "currently connected"
// files — it returns the single file this source was constructed for.
func (r *RESTSource) Files(ctx context.Context) ([]File, error) {
	if r.fileKey == "" {
		return nil, fmt.Errorf("REST source has no fileKey configured (figma-map.yaml: fileKey)")
	}
	raw, err := r.get(ctx, "/v1/files/"+r.fileKey, url.Values{"depth": {"1"}})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode file: %w", err)
	}
	return []File{{FileKey: r.fileKey, FileName: resp.Name}}, nil
}

// Document implements Source. REST returns the whole file (every page),
// not a "current page" — that's an editor-state concept with no REST
// equivalent, so the caller gets the full document root instead.
func (r *RESTSource) Document(ctx context.Context, fileKey string) (*Node, error) {
	return r.DocumentWithDepth(ctx, fileKey, 0)
}

// DocumentWithDepth implements Source.
func (r *RESTSource) DocumentWithDepth(ctx context.Context, fileKey string, depth int) (*Node, error) {
	q := url.Values{}
	if depth > 0 {
		q.Set("depth", strconv.Itoa(depth))
	}
	raw, err := r.get(ctx, "/v1/files/"+fileKey, q)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Document restNode `json:"document"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}
	node := resp.Document.toDomain()
	return &node, nil
}

// Node implements Source.
func (r *RESTSource) Node(ctx context.Context, fileKey, id string) (*Node, error) {
	return r.NodeWithDepth(ctx, fileKey, id, 0)
}

// NodeWithDepth implements Source.
func (r *RESTSource) NodeWithDepth(ctx context.Context, fileKey, id string, depth int) (*Node, error) {
	resp, err := r.fetchNodes(ctx, fileKey, []string{id}, depth)
	if err != nil {
		return nil, err
	}
	entry, ok := resp.Nodes[id]
	if !ok {
		return nil, fmt.Errorf("node %s not found", id)
	}
	node := entry.Document.toDomain()
	return &node, nil
}

// Selection implements Source. The Figma REST API has no concept of "what's
// currently selected in the editor" — that's a live-session concept, not
// data a static file snapshot carries. Returns an error rather than an
// empty slice, so a caller can't mistake "not supported" for "nothing
// selected".
func (r *RESTSource) Selection(_ context.Context, _ string) ([]Node, error) {
	return nil, fmt.Errorf("editor selection is not available via the Figma REST source (no live editor session) — use the bridge source instead")
}

// Screenshot implements Source.
func (r *RESTSource) Screenshot(ctx context.Context, fileKey, id string, opts ScreenshotOpts) ([]byte, error) {
	q := url.Values{"ids": {id}}
	format := opts.Format
	if format == "" {
		format = "PNG"
	}
	q.Set("format", strings.ToLower(format))
	if opts.Scale > 0 {
		q.Set("scale", strconv.FormatFloat(opts.Scale, 'g', -1, 64))
	}

	raw, err := r.get(ctx, "/v1/images/"+fileKey, q)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Images map[string]string `json:"images"`
		Err    *string           `json:"err"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode image export: %w", err)
	}
	if resp.Err != nil {
		return nil, fmt.Errorf("figma image export: %s", *resp.Err)
	}
	imgURL, ok := resp.Images[id]
	if !ok || imgURL == "" {
		return nil, fmt.Errorf("no image returned for node %s", id)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, err
	}
	imgResp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch exported image: %w", err)
	}
	defer func() { _ = imgResp.Body.Close() }()
	if imgResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch exported image returned %d", imgResp.StatusCode)
	}
	return io.ReadAll(imgResp.Body)
}

// Metadata implements Source. REST has no "current page" concept (see
// Document); CurrentPageID/CurrentPageName are left empty rather than
// guessed.
func (r *RESTSource) Metadata(ctx context.Context, fileKey string) (Metadata, error) {
	raw, err := r.get(ctx, "/v1/files/"+fileKey, url.Values{"depth": {"1"}})
	if err != nil {
		return Metadata{}, err
	}
	var resp struct {
		Name     string `json:"name"`
		Document struct {
			Children []restNode `json:"children"`
		} `json:"document"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	pages := make([]Page, 0, len(resp.Document.Children))
	for _, c := range resp.Document.Children {
		if c.Type == "CANVAS" {
			pages = append(pages, Page{ID: c.ID, Name: c.Name})
		}
	}
	return Metadata{FileName: resp.Name, Pages: pages}, nil
}

// VariableDefs implements Source. Enterprise/Dev-Mode-gated on Figma's side
// — a plan without access gets a clear wrapped error, not a silent empty
// catalog.
func (r *RESTSource) VariableDefs(ctx context.Context, fileKey string) (VariableDefs, error) {
	raw, err := r.get(ctx, "/v1/files/"+fileKey+"/variables/local", nil)
	if err != nil {
		return VariableDefs{}, fmt.Errorf("variables/local (requires Enterprise/Dev Mode access): %w", err)
	}
	var resp struct {
		Meta struct {
			Variables map[string]struct {
				ID                   string            `json:"id"`
				Name                 string            `json:"name"`
				ResolvedType         string            `json:"resolvedType"`
				VariableCollectionID string            `json:"variableCollectionId"`
				ValuesByMode         map[string]any    `json:"valuesByMode"`
				CodeSyntax           map[string]string `json:"codeSyntax,omitempty"`
				Scopes               []string          `json:"scopes,omitempty"`
			} `json:"variables"`
			VariableCollections map[string]struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Modes []struct {
					ModeID string `json:"modeId"`
					Name   string `json:"name"`
				} `json:"modes"`
			} `json:"variableCollections"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return VariableDefs{}, fmt.Errorf("decode variables/local: %w", err)
	}

	byCollection := map[string][]Variable{}
	for _, v := range resp.Meta.Variables {
		byCollection[v.VariableCollectionID] = append(byCollection[v.VariableCollectionID], Variable{
			ID:           v.ID,
			Name:         v.Name,
			ResolvedType: v.ResolvedType,
			ValuesByMode: v.ValuesByMode,
			CodeSyntax:   v.CodeSyntax,
			Scopes:       v.Scopes,
		})
	}

	var defs VariableDefs
	for id, c := range resp.Meta.VariableCollections {
		modes := make([]VariableMode, 0, len(c.Modes))
		for _, m := range c.Modes {
			modes = append(modes, VariableMode{ModeID: m.ModeID, Name: m.Name})
		}
		defs.Collections = append(defs.Collections, VariableCollection{
			ID:        id,
			Name:      c.Name,
			Modes:     modes,
			Variables: byCollection[id],
		})
	}
	return defs, nil
}

// FindNodes implements Source. REST has no server-side search — this walks
// the file's full tree (or opts.WithinNodeID's subtree) client-side. Fine
// for the headless/CI use case this source targets; a large file pays the
// cost of one full-tree fetch, same as the bridge would for an unfiltered
// get_document.
func (r *RESTSource) FindNodes(ctx context.Context, fileKey string, opts FindNodesOptions) ([]FindMatch, error) {
	var root *Node
	var err error
	if opts.WithinNodeID != "" {
		root, err = r.Node(ctx, fileKey, opts.WithinNodeID)
	} else {
		root, err = r.Document(ctx, fileKey)
	}
	if err != nil {
		return nil, err
	}

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}
	var matches []FindMatch
	var walk func(n *Node, path string, depth int)
	walk = func(n *Node, path string, depth int) {
		if len(matches) >= maxResults {
			return
		}
		if opts.MaxDepth > 0 && depth > opts.MaxDepth {
			return
		}
		if depth > 0 && nodeMatches(n, opts) {
			matches = append(matches, FindMatch{
				ID:             n.ID,
				Name:           n.Name,
				Type:           n.Type,
				Path:           path,
				Characters:     n.Characters,
				ComponentProps: n.ComponentProps,
			})
		}
		childPath := n.Name
		if path != "" {
			childPath = path + " › " + n.Name
		}
		for i := range n.Children {
			if len(matches) >= maxResults {
				return
			}
			walk(&n.Children[i], childPath, depth+1)
		}
	}
	walk(root, "", 0)
	return matches, nil
}

func nodeMatches(n *Node, opts FindNodesOptions) bool {
	if opts.Query != "" && !strings.Contains(strings.ToLower(n.Name), strings.ToLower(opts.Query)) {
		return false
	}
	if opts.NodeType != "" && !strings.EqualFold(n.Type, opts.NodeType) {
		return false
	}
	if opts.TextQuery != "" && !strings.Contains(strings.ToLower(n.Characters), strings.ToLower(opts.TextQuery)) {
		return false
	}
	return true
}

// MainComponentName implements Source. Resolves via the /nodes endpoint's
// top-level components/componentSets maps, which describe every
// component/set referenced in the returned subtree — no extra request
// needed beyond the one that already fetches the instance.
func (r *RESTSource) MainComponentName(ctx context.Context, fileKey, id string) (string, error) {
	resp, err := r.fetchNodes(ctx, fileKey, []string{id}, 1)
	if err != nil {
		return "", err
	}
	entry, ok := resp.Nodes[id]
	if !ok || entry.Document.ComponentID == "" {
		return "", nil
	}
	comp, ok := resp.Components[entry.Document.ComponentID]
	if !ok {
		return "", nil
	}
	if comp.ComponentSetID != "" {
		if set, ok := resp.ComponentSets[comp.ComponentSetID]; ok {
			return set.Name, nil
		}
	}
	return comp.Name, nil
}

// Animation implements Source. Resolving a reaction's before/after style
// delta needs either a live editor session (for the variant-sibling guess,
// which the bridge's plugin runtime does inline) or meaningfully more
// REST plumbing than figma-map's REST source currently carries (walking
// componentSets plus a second /nodes fetch for the destination) — not worth
// building until a REST-only workflow actually needs it. Use the bridge
// source for `figma animation` today.
func (r *RESTSource) Animation(_ context.Context, _, _ string) ([]Animation, error) {
	return nil, fmt.Errorf("animation resolution is not available via the Figma REST source — use the bridge source instead")
}

type restNodesResponse struct {
	Nodes map[string]struct {
		Document restNode `json:"document"`
	} `json:"nodes"`
	Components    map[string]restComponent `json:"components"`
	ComponentSets map[string]restComponent `json:"componentSets"`
}

type restComponent struct {
	Name           string `json:"name"`
	ComponentSetID string `json:"componentSetId,omitempty"`
}

func (r *RESTSource) fetchNodes(ctx context.Context, fileKey string, ids []string, depth int) (*restNodesResponse, error) {
	q := url.Values{"ids": {strings.Join(ids, ",")}}
	if depth > 0 {
		q.Set("depth", strconv.Itoa(depth))
	}
	raw, err := r.get(ctx, "/v1/files/"+fileKey+"/nodes", q)
	if err != nil {
		return nil, err
	}
	var resp restNodesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode nodes: %w", err)
	}
	return &resp, nil
}

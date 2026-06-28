package service

import (
	"context"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// FindResult is one matching Figma node returned by Find.
type FindResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	// Path is the breadcrumb from page root to this node, e.g. "Page › Frame › Group".
	Path string `json:"path"`
	// Text is the node's text content (TEXT nodes only).
	Text string `json:"text,omitempty"`
	// VariantModes holds explicit variable mode overrides, e.g. {"Color Semantic": "Dark"}.
	VariantModes map[string]string `json:"variantModes,omitempty"`
	// ComponentProps holds variant property values for INSTANCE nodes.
	ComponentProps map[string]any `json:"componentProps,omitempty"`
	// DevStatus is "READY_FOR_DEV" or "COMPLETED" when set (top-level frames
	// only) — a discovery filter for which frames are actually ready to build.
	DevStatus string `json:"devStatus,omitempty"`
}

// FindResults is the `find` output.
type FindResults struct {
	Nodes []FindResult `json:"nodes"`
}

// FindOptions controls what Find matches on.
type FindOptions struct {
	// Query matches against node name (case-insensitive substring). Empty = match all.
	Query string
	// TextQuery additionally requires the node's text content to contain this string.
	TextQuery string
	// NodeType filters to nodes of this Figma type (e.g. "FRAME", "TEXT", "INSTANCE").
	// Empty means all types.
	NodeType string
	// Mode filters to nodes that have a variable mode override whose name contains this string,
	// e.g. "Dark" matches {"Color Semantic": "Dark"}.
	Mode string
	// WithinNodeID restricts the search to a specific node subtree.
	// Empty means search the entire document.
	WithinNodeID string
	// Depth caps how deep the WithinNodeID subtree is searched (0 =
	// unlimited). Has no effect on a whole-document search.
	Depth int
	// MaxResults caps the output. 0 = default 50.
	MaxResults int
}

// Find searches the Figma document tree for nodes matching opts. Matching
// happens inside the Figma plugin sandbox (see figma.Source.FindNodes) — the
// only way to search a whole document without paying full style/variable
// serialization for every node, which is what made `find` time out before.
func (s *Service) Find(ctx context.Context, fileKey string, opts FindOptions) (FindResults, error) {
	if opts.MaxResults <= 0 {
		opts.MaxResults = 50
	}

	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return FindResults{}, err
	}

	matches, err := s.src.FindNodes(ctx, key, figma.FindNodesOptions{
		Query:        opts.Query,
		TextQuery:    opts.TextQuery,
		NodeType:     opts.NodeType,
		Mode:         opts.Mode,
		WithinNodeID: opts.WithinNodeID,
		MaxDepth:     opts.Depth,
		MaxResults:   opts.MaxResults,
	})
	if err != nil {
		return FindResults{}, err
	}

	results := make([]FindResult, len(matches))
	for i, m := range matches {
		results[i] = FindResult{
			ID:             m.ID,
			Name:           m.Name,
			Type:           m.Type,
			Path:           m.Path,
			Text:           m.Characters,
			VariantModes:   m.VariantModes,
			ComponentProps: m.ComponentProps,
			DevStatus:      m.DevStatus,
		}
	}
	return FindResults{Nodes: results}, nil
}

package service

import (
	"context"
	"strings"

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
	// MaxResults caps the output. 0 = default 50.
	MaxResults int
}

// Find searches the full Figma document tree for nodes matching opts.
func (s *Service) Find(ctx context.Context, fileKey string, opts FindOptions) (FindResults, error) {
	if opts.MaxResults <= 0 {
		opts.MaxResults = 50
	}

	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return FindResults{}, err
	}

	var root *figma.Node
	if opts.WithinNodeID != "" {
		root, err = s.src.Node(ctx, key, opts.WithinNodeID)
	} else {
		root, err = s.src.Document(ctx, key)
	}
	if err != nil {
		return FindResults{}, err
	}

	var results []FindResult
	walkFind(root, "", opts, &results)
	return FindResults{Nodes: results}, nil
}

func walkFind(n *figma.Node, parentPath string, opts FindOptions, out *[]FindResult) {
	if len(*out) >= opts.MaxResults {
		return
	}

	path := parentPath
	if n.Name != "" {
		if path == "" {
			path = n.Name
		} else {
			path = parentPath + " › " + n.Name
		}
	}

	// Skip the root page node itself — only match its descendants.
	if n.Type != "CANVAS" && n.Type != "DOCUMENT" {
		if matchesFind(n, opts) {
			*out = append(*out, FindResult{
				ID:             n.ID,
				Name:           n.Name,
				Type:           n.Type,
				Path:           parentPath,
				Text:           n.Characters,
				VariantModes:   n.VariantModes,
				ComponentProps: n.ComponentProps,
				DevStatus:      n.DevStatus,
			})
		}
	}

	for i := range n.Children {
		if len(*out) >= opts.MaxResults {
			return
		}
		walkFind(&n.Children[i], path, opts, out)
	}
}

func matchesFind(n *figma.Node, opts FindOptions) bool {
	if opts.Query != "" && !containsCI(n.Name, opts.Query) {
		return false
	}
	if opts.NodeType != "" && !strings.EqualFold(n.Type, opts.NodeType) {
		return false
	}
	if opts.TextQuery != "" && !containsCI(n.Characters, opts.TextQuery) {
		return false
	}
	if opts.Mode != "" {
		found := false
		for _, v := range n.VariantModes {
			if containsCI(v, opts.Mode) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsCI(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

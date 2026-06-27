package service

import (
	"context"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// InspectNode is one node in a flattened subtree view. The tree is flattened
// (depth + parentId) rather than nested so it serializes without recursion and
// is easy for an agent to iterate.
type InspectNode struct {
	ID       string       `json:"id"`
	ParentID string       `json:"parentId,omitempty"`
	Depth    int          `json:"depth"`
	Name     string       `json:"name"`
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	Bounds   figma.Bounds `json:"bounds"`
	Tokens   *Tokens      `json:"tokens,omitempty"`
	// Reactions are this node's prototyping reactions — opt-in detail, only
	// populated alongside Tokens (withTokens=true).
	Reactions []figma.Reaction `json:"reactions,omitempty"`
	// DevResources/Annotations are designer-attached links and notes — also
	// opt-in, never auto-applied, but a strong human-given hint to read.
	DevResources []figma.DevResource `json:"devResources,omitempty"`
	Annotations  []string            `json:"annotations,omitempty"`
}

// InspectResult is the `inspect` output: a flat, pre-order list of nodes.
type InspectResult struct {
	Nodes []InspectNode `json:"nodes"`
}

// Inspect returns a node's subtree as a flat list (structure + optional tokens).
// Deterministic. depth limits recursion: 0 means unlimited.
func (s *Service) Inspect(ctx context.Context, fileKey, nodeID string, withTokens bool, depth int) (InspectResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return InspectResult{}, err
	}
	node, err := s.src.Node(ctx, key, nodeID)
	if err != nil {
		return InspectResult{}, err
	}
	var res InspectResult
	flatten(&res, node, "", 0, withTokens, depth)
	return res, nil
}

func flatten(res *InspectResult, n *figma.Node, parentID string, cur int, withTokens bool, maxDepth int) {
	item := InspectNode{
		ID: n.ID, ParentID: parentID, Depth: cur,
		Name: n.Name, Type: n.Type, Text: n.Characters, Bounds: n.Bounds,
	}
	if withTokens {
		item.Tokens = tokensFromStyle(n.Styles)
		item.Reactions = n.Reactions
		item.DevResources = n.DevResources
		item.Annotations = n.Annotations
	}
	res.Nodes = append(res.Nodes, item)

	if maxDepth > 0 && cur >= maxDepth {
		return
	}
	for i := range n.Children {
		flatten(res, &n.Children[i], n.ID, cur+1, withTokens, maxDepth)
	}
}

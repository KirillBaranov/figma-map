package service

import (
	"context"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// SelectionNode is one currently-selected node in the Figma editor.
type SelectionNode struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	Text           string            `json:"text,omitempty"`
	Bounds         figma.Bounds      `json:"bounds"`
	VariantModes   map[string]string `json:"variantModes,omitempty"`
	ComponentProps map[string]any    `json:"componentProps,omitempty"`
}

// SelectionResult is the `selection` output.
type SelectionResult struct {
	Nodes []SelectionNode `json:"nodes"`
}

// Selection returns the nodes currently selected in the Figma editor.
// Empty when nothing is selected.
func (s *Service) Selection(ctx context.Context, fileKey string) (SelectionResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return SelectionResult{}, err
	}
	nodes, err := s.src.Selection(ctx, key)
	if err != nil {
		return SelectionResult{}, err
	}
	res := SelectionResult{Nodes: make([]SelectionNode, len(nodes))}
	for i, n := range nodes {
		res.Nodes[i] = SelectionNode{
			ID:             n.ID,
			Name:           n.Name,
			Type:           n.Type,
			Text:           n.Characters,
			Bounds:         n.Bounds,
			VariantModes:   n.VariantModes,
			ComponentProps: n.ComponentProps,
		}
	}
	return res, nil
}

package service

import (
	"context"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// AnimationResult is a node's prototyping reactions resolved to actual
// before/after style deltas — the data needed to write a real CSS
// transition/framer-motion animation instead of noting "this hovers" with
// no values. See figma.Animation for what each entry carries.
type AnimationResult struct {
	NodeID     string            `json:"nodeId"`
	Name       string            `json:"name"`
	Animations []figma.Animation `json:"animations"`
}

// GetAnimation resolves node's prototyping reactions to before/after style
// deltas. Unlike GetTokens' cheap Reactions field (trigger/timing only,
// carried for every node), this does the expensive destination-resolution/
// style-diff work — worth it only for the one node an agent is actually
// asking about.
func (s *Service) GetAnimation(ctx context.Context, fileKey, nodeID string) (AnimationResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return AnimationResult{}, err
	}
	node, err := s.src.Node(ctx, key, nodeID)
	if err != nil {
		return AnimationResult{}, err
	}
	animations, err := s.src.Animation(ctx, key, nodeID)
	if err != nil {
		return AnimationResult{}, err
	}
	return AnimationResult{NodeID: node.ID, Name: node.Name, Animations: animations}, nil
}

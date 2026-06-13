package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/llm"
	"github.com/kirillbaranov/figma-map/internal/render"
)

// tier2 screenshots the rendered URL and runs the encapsulated semantic check.
func (s *Service) tier2(ctx context.Context, key string, frame *figma.Node, url string, width int) ([]SemanticFinding, error) {
	rendered, err := render.Screenshot(ctx, url, width)
	if err != nil {
		return nil, err
	}
	return s.semanticDiff(ctx, key, frame, rendered)
}

const semanticPrompt = `You compare a DESIGN (Figma) against a RENDERED implementation.

Exact measurements (sizes, colors, spacing, fonts) are already checked
separately by deterministic tooling. Report ONLY what numbers cannot capture:
- elements present in the design but MISSING in the render (or vice versa: EXTRA)
- a wrong or absent icon / image / asset
- clearly wrong overall structure or arrangement

Return JSON only:
{ "findings": [ { "kind": "missing|extra|asset|structure", "detail": "...", "severity": "major|minor" } ] }
Return an empty array if the implementation faithfully reproduces the design's content and structure.`

// semanticDiff asks the vision LLM for content/structure differences the
// deterministic tier cannot measure. Encapsulated: callers see SemanticFinding,
// not the LLM.
func (s *Service) semanticDiff(ctx context.Context, key string, frame *figma.Node, rendered []byte) ([]SemanticFinding, error) {
	client, err := s.llmClient()
	if err != nil {
		return nil, err
	}
	design, err := s.bridge.Screenshot(key, frame.ID, figma.ScreenshotOpts{Scale: 2})
	if err != nil {
		return nil, err
	}

	reply, err := client.Vision(ctx, semanticPrompt, []llm.Image{
		{Label: "DESIGN:", PNG: design},
		{Label: "RENDERED:", PNG: rendered},
	})
	if err != nil {
		return nil, err
	}
	m := jsonObjRe.FindString(reply)
	if m == "" {
		return nil, fmt.Errorf("no JSON in reply")
	}
	var parsed struct {
		Findings []SemanticFinding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(m), &parsed); err != nil {
		return nil, fmt.Errorf("parse semantic findings: %w", err)
	}
	return parsed.Findings, nil
}

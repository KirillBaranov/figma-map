package service

import (
	"context"
	"fmt"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/llm"
	"github.com/kirillbaranov/figma-map/internal/render"
)

// CropRegion is a rectangle in the frame's own coordinate space — the space
// both the Figma screenshot (scale 1) and the rendered screenshot share,
// since the rendered viewport is sized to the frame's width. Used to scope
// the VLM tier to a handful of unresolved regions instead of the whole
// frame (see semanticDiff).
type CropRegion struct {
	X, Y, W, H int
}

// semanticCropBudget bounds how many crops get their own VLM call — one call
// per crop, so this is a direct cost/latency cap, not just a display limit.
const semanticCropBudget = 5

// tier2 screenshots the rendered URL and runs the encapsulated semantic
// check. crops, when non-empty, scope the check to those regions only
// (capped at semanticCropBudget) instead of the whole frame — see
// semanticDiff.
func (s *Service) tier2(ctx context.Context, key string, frame *figma.Node, url string, width int, crops []CropRegion) ([]SemanticFinding, error) {
	rendered, err := render.Screenshot(ctx, url, width)
	if err != nil {
		return nil, err
	}
	return s.semanticDiff(ctx, key, frame, rendered, crops)
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
// deterministic tier cannot measure. Encapsulated: callers see
// SemanticFinding, not the LLM.
//
// crops empty: one call over the whole frame (the original, still-default
// behavior). crops non-empty: one call per crop (capped at
// semanticCropBudget), each seeing only that region of both images — the
// doc's "only unresolved clusters, only small crops" requirement. Findings
// from a scoped call are prefixed with the region so the agent can tell
// which part of the frame a finding refers to.
func (s *Service) semanticDiff(ctx context.Context, key string, frame *figma.Node, rendered []byte, crops []CropRegion) ([]SemanticFinding, error) {
	client, err := s.llmClient()
	if err != nil {
		return nil, err
	}
	design, err := s.src.Screenshot(ctx, key, frame.ID, figma.ScreenshotOpts{Scale: 2})
	if err != nil {
		return nil, err
	}

	if len(crops) == 0 {
		return runSemanticCall(ctx, client, design, rendered, "")
	}

	if len(crops) > semanticCropBudget {
		crops = crops[:semanticCropBudget]
	}
	var findings []SemanticFinding
	for _, c := range crops {
		// design was captured at Scale:2, rendered at the frame's own
		// pixel scale — the crop rect is in frame space, so it's scaled ×2
		// for the design image only.
		designCrop, err := render.CropPNG(design, c.X*2, c.Y*2, c.W*2, c.H*2)
		if err != nil {
			return nil, fmt.Errorf("crop design region %+v: %w", c, err)
		}
		renderedCrop, err := render.CropPNG(rendered, c.X, c.Y, c.W, c.H)
		if err != nil {
			return nil, fmt.Errorf("crop rendered region %+v: %w", c, err)
		}
		label := fmt.Sprintf("region (%d,%d,%dx%d)", c.X, c.Y, c.W, c.H)
		cropFindings, err := runSemanticCall(ctx, client, designCrop, renderedCrop, label)
		if err != nil {
			return nil, err
		}
		findings = append(findings, cropFindings...)
	}
	return findings, nil
}

// runSemanticCall makes one VisionJSON call for a design/rendered image
// pair, prefixing each finding's Detail with regionLabel when scoped to a
// crop (regionLabel == "" for the whole-frame call).
func runSemanticCall(ctx context.Context, client llm.VisionModel, design, rendered []byte, regionLabel string) ([]SemanticFinding, error) {
	var parsed struct {
		Findings []SemanticFinding `json:"findings"`
	}
	if err := client.VisionJSON(ctx, semanticPrompt, []llm.Image{
		{Label: "DESIGN:", PNG: design},
		{Label: "RENDERED:", PNG: rendered},
	}, "semantic_diff", &parsed); err != nil {
		return nil, err
	}
	if regionLabel != "" {
		for i := range parsed.Findings {
			parsed.Findings[i].Detail = fmt.Sprintf("[%s] %s", regionLabel, parsed.Findings[i].Detail)
		}
	}
	return parsed.Findings, nil
}

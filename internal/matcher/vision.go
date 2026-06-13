package matcher

import (
	"context"
	"fmt"

	"github.com/kirillbaranov/figma-map/internal/llm"
)

// Vision is a Matcher backed by a vision LLM. It sends the target image plus all
// candidate images in one call and asks for a scored verdict via structured
// output (no free-text JSON parsing).
type Vision struct {
	model llm.VisionModel
	// MinScore is the threshold below which Best is reported as NO MATCH.
	MinScore float64
}

// NewVision returns a Vision matcher with a default 0.5 NO-MATCH threshold.
func NewVision(model llm.VisionModel) *Vision {
	return &Vision{model: model, MinScore: 0.5}
}

// visionVerdict is the structured-output contract. Array-shaped (no maps) so it
// maps cleanly to a strict JSON schema.
type visionVerdict struct {
	Matches []struct {
		ID     string  `json:"id"`
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	} `json:"matches"`
	BestID     string `json:"best_id"`
	Confidence string `json:"confidence"`
	Notes      string `json:"notes"`
}

const matchPromptHeader = `You are a UI component matcher. You are given a TARGET design image (from Figma) and a set of CANDIDATE code-component screenshots (from Storybook), each preceded by a [id] marker.

Identify which candidate best matches the component shown in the TARGET. Match by component TYPE and visual structure, not by exact text content.

Score guide: 1.0 identical, >=0.7 same component type, 0.4-0.7 similar, <0.4 different. If the best score is below 0.5, set best_id to "". confidence is "high", "medium", or "low".`

// Match implements Matcher.
func (v *Vision) Match(ctx context.Context, target Target, candidates []CatalogItem) (Result, error) {
	if len(candidates) == 0 {
		return Result{}, fmt.Errorf("no candidates provided")
	}

	prompt := matchPromptHeader
	if target.Name != "" || target.Label != "" {
		prompt += fmt.Sprintf("\n\nTARGET context — figma layer name: %q, text label: %q (hints only).", target.Name, target.Label)
	}
	prompt += "\n\nTARGET image follows, then the CANDIDATES (each preceded by its [id]):"

	images := []llm.Image{{PNG: target.PNG}}
	for _, c := range candidates {
		images = append(images, llm.Image{Label: "[" + c.Story.ID + "]", PNG: c.PNG})
	}

	var verdict visionVerdict
	if err := v.model.VisionJSON(ctx, prompt, images, "component_match", &verdict); err != nil {
		return Result{}, err
	}
	return v.toResult(verdict, candidates), nil
}

// toResult maps a verdict back onto catalog stories and applies the NO-MATCH
// threshold.
func (v *Vision) toResult(verdict visionVerdict, candidates []CatalogItem) Result {
	byID := make(map[string]CatalogItem, len(candidates))
	for _, c := range candidates {
		byID[c.Story.ID] = c
	}

	res := Result{Confidence: verdict.Confidence, Notes: verdict.Notes}
	for _, m := range verdict.Matches {
		if item, ok := byID[m.ID]; ok {
			res.Candidates = append(res.Candidates, Candidate{Story: item.Story, Score: m.Score, Reason: m.Reason})
		}
	}

	if best, ok := byID[verdict.BestID]; ok {
		if score := scoreOf(verdict, verdict.BestID); score >= v.MinScore {
			res.Best = &Candidate{Story: best.Story, Score: score}
		}
	}
	return res
}

func scoreOf(v visionVerdict, id string) float64 {
	for _, m := range v.Matches {
		if m.ID == id {
			return m.Score
		}
	}
	return 0
}

package matcher

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/llm"
)

// Vision is a Matcher backed by a vision LLM. It sends the target image plus
// all candidate images in one call and asks for a scored JSON verdict.
type Vision struct {
	client *llm.Client
	// MinScore is the threshold below which Best is reported as NO MATCH.
	MinScore float64
}

// NewVision returns a Vision matcher with a default 0.5 NO-MATCH threshold.
func NewVision(client *llm.Client) *Vision {
	return &Vision{client: client, MinScore: 0.5}
}

// visionVerdict is the JSON contract the model is asked to return.
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

Return JSON only, no prose:
{
  "matches": [ { "id": "candidate-id", "score": 0.0-1.0, "reason": "brief" } ],
  "best_id": "candidate-id or empty string if no good match",
  "confidence": "high" | "medium" | "low",
  "notes": "short observation"
}

Score guide: 1.0 identical, >=0.7 same component type, 0.4-0.7 similar, <0.4 different. If the best score is below 0.5, set best_id to "".`

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

	reply, err := v.client.Vision(ctx, prompt, images)
	if err != nil {
		return Result{}, err
	}

	verdict, err := parseVerdict(reply)
	if err != nil {
		return Result{}, fmt.Errorf("%w (raw: %s)", err, truncate(reply, 200))
	}

	return v.toResult(verdict, candidates), nil
}

// toResult maps a parsed verdict back onto catalog stories and applies the
// NO-MATCH threshold.
func (v *Vision) toResult(verdict visionVerdict, candidates []CatalogItem) Result {
	byID := make(map[string]CatalogItem, len(candidates))
	for _, c := range candidates {
		byID[c.Story.ID] = c
	}

	res := Result{Confidence: verdict.Confidence, Notes: verdict.Notes}
	for _, m := range verdict.Matches {
		item, ok := byID[m.ID]
		if !ok {
			continue
		}
		res.Candidates = append(res.Candidates, Candidate{
			Story:  item.Story,
			Score:  m.Score,
			Reason: m.Reason,
		})
	}

	if best, ok := byID[verdict.BestID]; ok {
		score := scoreOf(verdict, verdict.BestID)
		if score >= v.MinScore {
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

// jsonRe extracts the first {...} block from a possibly fenced reply.
var jsonRe = regexp.MustCompile(`(?s)\{.*\}`)

func parseVerdict(reply string) (visionVerdict, error) {
	var verdict visionVerdict
	m := jsonRe.FindString(reply)
	if m == "" {
		return verdict, fmt.Errorf("no JSON object in reply")
	}
	if err := json.Unmarshal([]byte(m), &verdict); err != nil {
		return verdict, fmt.Errorf("parse verdict: %w", err)
	}
	return verdict, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

package matcher

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/llm"
	"github.com/kirillbaranov/figma-map/internal/storybook"
)

// stubModel replays one canned JSON response into the structured output.
type stubModel struct{ json string }

func (m stubModel) VisionJSON(_ context.Context, _ string, _ []llm.Image, _ string, out any) error {
	return json.Unmarshal([]byte(m.json), out)
}

func candidates() []CatalogItem {
	return []CatalogItem{
		{Story: storybook.Story{ID: "ui-button--primary", Component: "Button"}},
		{Story: storybook.Story{ID: "ui-input--default", Component: "Input"}},
	}
}

func TestVisionMatch_PicksBest(t *testing.T) {
	m := stubModel{json: `{"matches":[
		{"id":"ui-button--primary","score":0.9,"reason":"button"},
		{"id":"ui-input--default","score":0.2,"reason":"no"}],
		"best_id":"ui-button--primary","confidence":"high","notes":""}`}
	res, err := NewVision(m).Match(context.Background(), Target{Name: "btn"}, candidates())
	if err != nil {
		t.Fatal(err)
	}
	if res.Best == nil || res.Best.Story.Component != "Button" || res.Best.Score != 0.9 {
		t.Errorf("best = %+v", res.Best)
	}
	if len(res.Candidates) != 2 {
		t.Errorf("want 2 candidates, got %d", len(res.Candidates))
	}
}

func TestVisionMatch_NoMatchBelowThreshold(t *testing.T) {
	// best_id set but score under MinScore (0.5) → reported as no match.
	m := stubModel{json: `{"matches":[{"id":"ui-button--primary","score":0.3,"reason":"meh"}],
		"best_id":"ui-button--primary","confidence":"low","notes":""}`}
	res, err := NewVision(m).Match(context.Background(), Target{}, candidates())
	if err != nil {
		t.Fatal(err)
	}
	if res.Best != nil {
		t.Errorf("expected no match below threshold, got %+v", res.Best)
	}
}

func TestVisionMatch_EmptyBestID(t *testing.T) {
	m := stubModel{json: `{"matches":[],"best_id":"","confidence":"low","notes":"nothing"}`}
	res, err := NewVision(m).Match(context.Background(), Target{}, candidates())
	if err != nil {
		t.Fatal(err)
	}
	if res.Best != nil {
		t.Errorf("expected nil best, got %+v", res.Best)
	}
}

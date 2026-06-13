package service

import (
	"context"
	"strings"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/figma"
)

// TestMapOrchestration exercises the full Map flow offline: fake Figma source +
// mock vision model + on-disk catalog/binding → JSX.
func TestMapOrchestration(t *testing.T) {
	catalogDir, bindingPath := tempCatalogBinding(t)

	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		png:   tinyPNG(t),
		nodes: map[string]*figma.Node{
			"1:1": {ID: "1:1", Type: "INSTANCE", Name: "button", Children: []figma.Node{
				{Type: "TEXT", Characters: "Continue"},
			}},
		},
	}
	mock := &mockModel{responses: map[string]string{
		"component_match": `{"matches":[{"id":"ui-button--primary","score":1.0,"reason":"x"}],"best_id":"ui-button--primary","confidence":"high","notes":""}`,
		"prop_values":     `{"values":[{"prop":"variant","value":"secondary"}]}`,
	}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	res, err := s.Map(context.Background(), "", bindingPath, catalogDir, "1:1")
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if res.Component != "Button" {
		t.Errorf("component = %q, want Button", res.Component)
	}
	if !strings.Contains(res.JSX, `import { Button } from "@/components/ui/button"`) {
		t.Errorf("missing import in JSX:\n%s", res.JSX)
	}
	if !strings.Contains(res.JSX, `variant="secondary"`) {
		t.Errorf("missing inferred prop in JSX:\n%s", res.JSX)
	}
	if !strings.Contains(res.JSX, ">Continue<") {
		t.Errorf("missing text in JSX:\n%s", res.JSX)
	}
}

// TestPlanDedup verifies Plan maps every instance but pays the LLM once per
// distinct instance (dedup by name+size).
func TestPlanDedup(t *testing.T) {
	catalogDir, bindingPath := tempCatalogBinding(t)

	inst := func(id, name string, w, h float64) figma.Node {
		return figma.Node{ID: id, Type: "INSTANCE", Name: name,
			Bounds: figma.Bounds{Width: w, Height: h}}
	}
	frame := &figma.Node{ID: "F", Type: "FRAME", Name: "Page",
		Bounds: figma.Bounds{Width: 1440, Height: 1024},
		Children: []figma.Node{
			inst("1", "button", 100, 40),
			inst("2", "button", 100, 40), // identical to #1 → dedup
			inst("3", "card", 200, 100),  // distinct
		}}

	fake := &fakeSource{
		files: []figma.File{{FileKey: "k"}},
		png:   tinyPNG(t),
		nodes: map[string]*figma.Node{"F": frame},
	}
	mock := &mockModel{responses: map[string]string{
		"component_match": `{"matches":[{"id":"ui-button--primary","score":0.9,"reason":"x"}],"best_id":"ui-button--primary","confidence":"high","notes":""}`,
		"prop_values":     `{"values":[]}`,
	}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	plan, err := s.Plan(context.Background(), "", "F", 0, bindingPath, catalogDir)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Components) != 3 {
		t.Errorf("want 3 mapped components, got %d", len(plan.Components))
	}
	if len(plan.Unmapped) != 0 {
		t.Errorf("want 0 unmapped, got %d", len(plan.Unmapped))
	}
	// Dedup: 2 distinct instances → matched twice, not three times.
	if got := mock.byName["component_match"]; got != 2 {
		t.Errorf("component_match calls = %d, want 2 (dedup of identical instances)", got)
	}
}

// TestPlanUnmapped: when the model finds no match, the instance is reported
// unmapped (never dropped).
func TestPlanUnmapped(t *testing.T) {
	catalogDir, bindingPath := tempCatalogBinding(t)
	frame := &figma.Node{ID: "F", Type: "FRAME", Name: "Page",
		Children: []figma.Node{{ID: "x", Type: "INSTANCE", Name: "mystery",
			Bounds: figma.Bounds{Width: 50, Height: 50}}}}
	fake := &fakeSource{files: []figma.File{{FileKey: "k"}}, png: tinyPNG(t),
		nodes: map[string]*figma.Node{"F": frame}}
	mock := &mockModel{responses: map[string]string{
		"component_match": `{"matches":[],"best_id":"","confidence":"low","notes":""}`,
	}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	plan, err := s.Plan(context.Background(), "", "F", 0, bindingPath, catalogDir)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Components) != 0 || len(plan.Unmapped) != 1 {
		t.Errorf("want 0 mapped / 1 unmapped, got %d / %d", len(plan.Components), len(plan.Unmapped))
	}
}

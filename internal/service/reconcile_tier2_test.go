package service

import (
	"context"
	"strings"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

func TestSemanticDiff_NoCropsWholeFrame(t *testing.T) {
	// crops == nil must preserve the original, single whole-frame call.
	fake := &fakeSource{png: tinyPNG(t)}
	mock := &mockModel{responses: map[string]string{
		"semantic_diff": `{"findings":[{"kind":"missing","detail":"icon absent","severity":"major"}]}`,
	}}
	s := &Service{src: fake, llm: mock}
	frame := &figma.Node{ID: "1:1"}

	findings, err := s.semanticDiff(context.Background(), "key", frame, tinyPNG(t), nil)
	if err != nil {
		t.Fatalf("semanticDiff: %v", err)
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 VLM call for whole-frame, got %d", mock.calls)
	}
	if len(findings) != 1 || strings.HasPrefix(findings[0].Detail, "[region") {
		t.Errorf("whole-frame finding should not carry a region prefix, got %+v", findings)
	}
}

func TestSemanticDiff_CropsCallPerRegion(t *testing.T) {
	fake := &fakeSource{png: tinyPNG(t)}
	mock := &mockModel{responses: map[string]string{
		"semantic_diff": `{"findings":[{"kind":"asset","detail":"wrong icon","severity":"minor"}]}`,
	}}
	s := &Service{src: fake, llm: mock}
	frame := &figma.Node{ID: "1:1"}
	crops := []CropRegion{{X: 0, Y: 0, W: 40, H: 40}, {X: 100, Y: 100, W: 20, H: 20}}

	findings, err := s.semanticDiff(context.Background(), "key", frame, tinyPNG(t), crops)
	if err != nil {
		t.Fatalf("semanticDiff: %v", err)
	}
	if mock.calls != 2 {
		t.Errorf("expected 1 VLM call per crop (2 crops), got %d", mock.calls)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (1 per crop), got %d", len(findings))
	}
	for _, f := range findings {
		if !strings.HasPrefix(f.Detail, "[region (") {
			t.Errorf("scoped finding should carry a region prefix, got %q", f.Detail)
		}
	}
}

func TestSemanticDiff_CropBudgetCap(t *testing.T) {
	fake := &fakeSource{png: tinyPNG(t)}
	mock := &mockModel{responses: map[string]string{
		"semantic_diff": `{"findings":[]}`,
	}}
	s := &Service{src: fake, llm: mock}
	frame := &figma.Node{ID: "1:1"}

	var crops []CropRegion
	for i := 0; i < semanticCropBudget+3; i++ {
		crops = append(crops, CropRegion{X: i * 10, Y: 0, W: 10, H: 10})
	}

	if _, err := s.semanticDiff(context.Background(), "key", frame, tinyPNG(t), crops); err != nil {
		t.Fatalf("semanticDiff: %v", err)
	}
	if mock.calls != semanticCropBudget {
		t.Errorf("expected calls capped at semanticCropBudget=%d, got %d", semanticCropBudget, mock.calls)
	}
}

func TestSemanticCrops_BuildsFromUnmeasuredAndSpatial(t *testing.T) {
	want := map[string]figmaTarget{
		"missing": {box: figma.Bounds{X: 0, Y: 0, Width: 40, Height: 40}},
		"spatial": {box: figma.Bounds{X: 100, Y: 100, Width: 20, Height: 20}},
		"decor":   {box: figma.Bounds{X: 200, Y: 200, Width: 10, Height: 10}},
		"zero":    {box: figma.Bounds{X: 300, Y: 300, Width: 0, Height: 0}},
	}
	unmeasured := []UnmeasuredNode{
		{NodeID: "missing", Actionable: true},
		{NodeID: "decor", Actionable: false}, // not actionable → excluded
		{NodeID: "no-box", Actionable: true}, // not in want → excluded
	}
	spatial := []string{"spatial", "zero"} // "zero" has no area → excluded

	crops := semanticCrops(want, unmeasured, spatial)
	if len(crops) != 2 {
		t.Fatalf("expected 2 crops, got %d: %+v", len(crops), crops)
	}
	byXY := map[[2]int]bool{}
	for _, c := range crops {
		byXY[[2]int{c.X, c.Y}] = true
	}
	if !byXY[[2]int{0, 0}] || !byXY[[2]int{100, 100}] {
		t.Errorf("expected crops at (0,0) and (100,100), got %+v", crops)
	}
}

package service

import (
	"testing"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/figma"
)

func TestCollectInstances(t *testing.T) {
	frame := &figma.Node{Type: "FRAME", Children: []figma.Node{
		{Type: "FRAME", Name: "wrap", Children: []figma.Node{
			{ID: "a", Type: "INSTANCE", Name: "a"},
		}},
		{ID: "b", Type: "INSTANCE", Name: "b", Children: []figma.Node{
			{ID: "nested", Type: "INSTANCE", Name: "nested"}, // must NOT be collected
		}},
	}}
	got := collectInstances(frame, 0)
	if len(got) != 2 {
		t.Fatalf("want 2 instances (a, b), got %d", len(got))
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids["a"] || !ids["b"] || ids["nested"] {
		t.Errorf("unexpected instances: %v", ids)
	}
}

func TestInstKeyDedup(t *testing.T) {
	a := &figma.Node{Name: "button", Bounds: figma.Bounds{Width: 100.2, Height: 40.4}}
	b := &figma.Node{Name: "button", Bounds: figma.Bounds{Width: 100, Height: 40}} // rounds equal
	c := &figma.Node{Name: "button", Bounds: figma.Bounds{Width: 200, Height: 40}}
	if instKey(a) != instKey(b) {
		t.Errorf("expected identical keys: %q vs %q", instKey(a), instKey(b))
	}
	if instKey(a) == instKey(c) {
		t.Error("expected different keys for different sizes")
	}
}

func TestLayoutOf(t *testing.T) {
	if layoutOf(nil) != nil {
		t.Error("nil style → nil layout")
	}
	st := &figma.Style{
		AutoLayout: &figma.AutoLayout{Direction: "HORIZONTAL", Gap: 12},
		Padding:    &figma.Padding{Top: 8},
	}
	l := layoutOf(st)
	if l == nil || l.Direction != "row" || l.Gap == nil || *l.Gap != 12 || l.Padding == nil {
		t.Errorf("layoutOf = %+v", l)
	}
}

func TestRenderMatchedJSX(t *testing.T) {
	comp := binding.Component{Symbol: "Button", Import: "../components/ui/button"}

	got := renderMatchedJSX(comp, map[string]string{"variant": "primary"}, "Start")
	want := `<Button variant="primary">Start</Button>`
	if got != want {
		t.Errorf("with text: got %q, want %q", got, want)
	}

	got = renderMatchedJSX(comp, nil, "")
	want = `<Button />`
	if got != want {
		t.Errorf("no props/text: got %q, want %q", got, want)
	}
}

func TestRenderUnmappedJSX(t *testing.T) {
	gen := &codeGen{}
	inst := &figma.Node{
		ID: "1:1", Type: "INSTANCE", Name: "Chip",
		Bounds: figma.Bounds{X: 240, Y: 80, Width: 64, Height: 24},
		Styles: &figma.Style{Fills: figma.MaybePaints{Paints: []figma.Paint{{Type: "SOLID", Color: "#e5e7eb"}}}},
	}
	got := renderUnmappedJSX(gen, inst)
	if got == "" {
		t.Fatal("expected non-empty JSX skeleton")
	}
	tree := gen.frame(&figma.Node{
		ID: "1:1", Type: "INSTANCE", Name: "Chip",
		Bounds: figma.Bounds{X: 0, Y: 0, Width: 64, Height: 24},
		Styles: &figma.Style{Fills: figma.MaybePaints{Paints: []figma.Paint{{Type: "SOLID", Color: "#e5e7eb"}}}},
	}, false, figma.Bounds{Width: 64, Height: 24})
	if want := renderJSX(t, tree); got != want {
		t.Errorf("expected root bounds reset to origin, got %q want %q", got, want)
	}
}

func TestCollectTargets(t *testing.T) {
	frame := &figma.Node{ID: "f", Type: "FRAME",
		Styles: &figma.Style{Fills: figma.MaybePaints{Paints: []figma.Paint{{Type: "SOLID", Color: "#fff"}}}},
		Children: []figma.Node{
			{ID: "t", Type: "TEXT", Styles: &figma.Style{FontSize: figma.MaybeNum{Value: 16, Set: true}}},
			{ID: "plain", Type: "FRAME"}, // no styles → not a target
		}}
	out := map[string]figmaTarget{}
	collectTargets(frame, 0, 0, true, false, 0, 0, out)
	if _, ok := out["f"]; !ok {
		t.Error("frame with fill should be a target")
	}
	if _, ok := out["t"]; !ok {
		t.Error("text with font should be a target")
	}
	if _, ok := out["plain"]; ok {
		t.Error("node without tokens should not be a target")
	}
}

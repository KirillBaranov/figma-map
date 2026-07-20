package service

import (
	"testing"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/render"
)

func TestIoU(t *testing.T) {
	a := box{0, 0, 1, 1}
	if v := iou(a, a); v != 1 {
		t.Errorf("identical IoU = %v, want 1", v)
	}
	if v := iou(box{0, 0, 1, 1}, box{2, 2, 1, 1}); v != 0 {
		t.Errorf("disjoint IoU = %v, want 0", v)
	}
	// Half-overlap on x: intersection 0.5, union 1.5 → 1/3.
	if v := iou(box{0, 0, 1, 1}, box{0.5, 0, 1, 1}); v < 0.33 || v > 0.34 {
		t.Errorf("half-overlap IoU = %v, want ~0.333", v)
	}
}

func TestAlign_PrefersExactTag(t *testing.T) {
	want := map[string]figmaTarget{
		"a": {typ: "FRAME", box: figma.Bounds{X: 0, Y: 0, Width: 100, Height: 100}},
	}
	els := []render.DOMElement{
		{FigmaNode: "a", Box: render.Box{X: 500, Y: 500, Width: 10, Height: 10}}, // tagged but far
		{Box: render.Box{X: 0, Y: 0, Width: 100, Height: 100}},                   // untagged, perfect geometry
	}
	matched, spatial := alignElements(want, els)
	if matched["a"].FigmaNode != "a" {
		t.Errorf("tag match should win over geometry, got %+v", matched["a"])
	}
	if len(spatial) != 0 {
		t.Errorf("no spatial matches expected, got %v", spatial)
	}
}

func TestAlign_SpatialFallback(t *testing.T) {
	// Two untagged design nodes aligned to two untagged DOM elements by position.
	want := map[string]figmaTarget{
		"hero": {typ: "FRAME", box: figma.Bounds{X: 0, Y: 0, Width: 200, Height: 100}},
		"btn":  {typ: "FRAME", box: figma.Bounds{X: 0, Y: 120, Width: 80, Height: 40}},
	}
	els := []render.DOMElement{
		{Tag: "div", Box: render.Box{X: 0, Y: 0, Width: 200, Height: 100}},
		{Tag: "div", Box: render.Box{X: 0, Y: 120, Width: 80, Height: 40}},
	}
	matched, spatial := alignElements(want, els)
	if len(matched) != 2 {
		t.Fatalf("want 2 matched, got %d", len(matched))
	}
	if len(spatial) != 2 {
		t.Errorf("both should be spatial, got %v", spatial)
	}
	// hero (bigger, top) should map to the 200x100 element.
	if matched["hero"].Box.Width != 200 || matched["btn"].Box.Height != 40 {
		t.Errorf("mis-aligned: hero=%+v btn=%+v", matched["hero"].Box, matched["btn"].Box)
	}
}

func TestAlign_TextBreaksTie(t *testing.T) {
	// Two identical boxes; the correct one is disambiguated by text.
	want := map[string]figmaTarget{
		"t": {typ: "TEXT", text: "Get started", box: figma.Bounds{X: 0, Y: 0, Width: 100, Height: 20}},
	}
	els := []render.DOMElement{
		{Tag: "span", Text: "Learn more", Box: render.Box{X: 0, Y: 0, Width: 100, Height: 20}},
		{Tag: "span", Text: "Get started", Box: render.Box{X: 0, Y: 0, Width: 100, Height: 20}},
	}
	matched, _ := alignElements(want, els)
	if matched["t"].Text != "Get started" {
		t.Errorf("text should break the tie, got %q", matched["t"].Text)
	}
}

func TestAlign_AccessibleTextBreaksTieForNonTextNode(t *testing.T) {
	// A text-less Figma instance ("Icon/Search") has no own text to anchor
	// on, but the correct DOM element carries a matching aria-label — this
	// anchor must apply to non-TEXT node types too (Phase D: previously only
	// TEXT nodes got the textOverlap bonus).
	want := map[string]figmaTarget{
		"icon": {typ: "INSTANCE", name: "Icon/Search", box: figma.Bounds{X: 0, Y: 0, Width: 24, Height: 24}},
	}
	els := []render.DOMElement{
		{Tag: "svg", AccessibleText: "Close", Box: render.Box{X: 0, Y: 0, Width: 24, Height: 24}},
		{Tag: "svg", AccessibleText: "Search", Box: render.Box{X: 0, Y: 0, Width: 24, Height: 24}},
	}
	matched, _ := alignElements(want, els)
	if matched["icon"].AccessibleText != "Search" {
		t.Errorf("aria-label should break the tie, got %q", matched["icon"].AccessibleText)
	}
}

func TestAlign_ClassNameBreaksTie(t *testing.T) {
	// No accessible text either, but the DOM class echoes the component name.
	want := map[string]figmaTarget{
		"btn": {typ: "INSTANCE", name: "Button/Primary", box: figma.Bounds{X: 0, Y: 0, Width: 100, Height: 40}},
	}
	els := []render.DOMElement{
		{Tag: "button", Class: "button-secondary", Box: render.Box{X: 0, Y: 0, Width: 100, Height: 40}},
		{Tag: "button", Class: "button-primary", Box: render.Box{X: 0, Y: 0, Width: 100, Height: 40}},
	}
	matched, _ := alignElements(want, els)
	if matched["btn"].Class != "button-primary" {
		t.Errorf("class should break the tie, got %q", matched["btn"].Class)
	}
}

func TestNormalizeAnchorName(t *testing.T) {
	cases := map[string]string{
		"Icon/Search":       "icon search",
		"button--primary":   "button primary",
		"  Card_Item.large": "card item large",
		"":                  "",
	}
	for in, want := range cases {
		if got := normalizeAnchorName(in); got != want {
			t.Errorf("normalizeAnchorName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAlign_NoMatchBelowThreshold(t *testing.T) {
	want := map[string]figmaTarget{
		"x": {typ: "FRAME", box: figma.Bounds{X: 0, Y: 0, Width: 10, Height: 10}},
	}
	els := []render.DOMElement{
		{Tag: "div", Box: render.Box{X: 900, Y: 900, Width: 10, Height: 10}}, // far corner
	}
	matched, _ := alignElements(want, els)
	if _, ok := matched["x"]; ok {
		t.Error("disjoint geometry should not match")
	}
}

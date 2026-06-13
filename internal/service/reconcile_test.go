package service

import (
	"testing"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/render"
)

func TestTier1_PerfectMatchWithinTolerance(t *testing.T) {
	want := map[string]figmaTarget{
		"1": {typ: "FRAME", name: "Card", tokens: &Tokens{
			Fill:    "#ffffff",
			Radius:  ptr(8.0),
			Gap:     ptr(16.0),
			Padding: &figma.Padding{Top: 16, Right: 16, Bottom: 16, Left: 16},
		}},
	}
	got := map[string]render.DOMElement{
		"1": {FigmaNode: "1", Styles: map[string]string{
			"background-color":       "rgb(255, 255, 255)",
			"border-top-left-radius": "7.7px", // sub-pixel: within tol → NOT a diff
			"gap":                    "16px",
			"padding-top":            "16px",
			"padding-right":          "16px",
			"padding-bottom":         "16px",
			"padding-left":           "16px",
		}},
	}
	byEl, unmeasured := tier1Diff(want, got)
	if len(byEl) != 0 {
		t.Errorf("expected no diffs within tolerance, got %+v", byEl)
	}
	if len(unmeasured) != 0 {
		t.Errorf("unexpected unmeasured: %v", unmeasured)
	}
}

func TestTier1_DetectsRealDifferences(t *testing.T) {
	want := map[string]figmaTarget{
		"btn": {typ: "FRAME", name: "Button", tokens: &Tokens{
			Fill:   "#18181b",
			Radius: ptr(8.0),
		}},
	}
	got := map[string]render.DOMElement{
		"btn": {FigmaNode: "btn", Styles: map[string]string{
			"background-color":       "rgb(31, 41, 55)", // wrong color
			"border-top-left-radius": "4px",             // 4 vs 8 → beyond tol
		}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 1 {
		t.Fatalf("expected 1 element diff, got %d", len(byEl))
	}
	props := map[string]FieldDiff{}
	for _, d := range byEl[0].Diffs {
		props[d.Prop] = d
	}
	if d, ok := props["background-color"]; !ok || d.Should != "#18181b" {
		t.Errorf("color diff missing/wrong: %+v", props)
	}
	if d, ok := props["border-radius"]; !ok || d.Is != "4px" || d.Should != "8px" {
		t.Errorf("radius diff missing/wrong: %+v", props)
	}
}

func TestTier1_TextNodeComparesColorAndFont(t *testing.T) {
	want := map[string]figmaTarget{
		"t": {typ: "TEXT", name: "Title", tokens: &Tokens{
			Fill: "#18181b", FontSize: ptr(24.0), FontWeight: ptr(600.0),
		}},
	}
	got := map[string]render.DOMElement{
		"t": {FigmaNode: "t", Styles: map[string]string{
			"color":       "rgb(24, 24, 27)", // matches #18181b
			"font-size":   "24px",
			"font-weight": "700", // 700 vs 600 → diff
		}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 1 || len(byEl[0].Diffs) != 1 || byEl[0].Diffs[0].Prop != "font-weight" {
		t.Fatalf("expected only font-weight diff, got %+v", byEl)
	}
}

func TestTier1_UnmeasuredWhenNoDOMMatch(t *testing.T) {
	want := map[string]figmaTarget{
		"ghost": {typ: "FRAME", name: "Ghost", tokens: &Tokens{Fill: "#fff"}},
	}
	byEl, unmeasured := tier1Diff(want, map[string]render.DOMElement{})
	if len(byEl) != 0 {
		t.Errorf("expected no diffs, got %+v", byEl)
	}
	if len(unmeasured) != 1 || unmeasured[0] != "ghost" {
		t.Errorf("expected ghost unmeasured, got %v", unmeasured)
	}
}

func TestCanonColor(t *testing.T) {
	cases := []struct {
		a, b string
		same bool
	}{
		{"#ffffff", "rgb(255, 255, 255)", true},
		{"#18181b", "rgb(24, 24, 27)", true},
		{"#fff", "rgb(255,255,255)", true},
		{"#000000", "rgb(0, 0, 0)", true},
		{"#18181b", "rgb(31, 41, 55)", false},
	}
	for _, c := range cases {
		ca, oka := canonColor(c.a)
		cb, okb := canonColor(c.b)
		if !oka || !okb {
			t.Errorf("canonColor failed to parse %q/%q", c.a, c.b)
			continue
		}
		if (ca == cb) != c.same {
			t.Errorf("canonColor(%q)=%q vs (%q)=%q, want same=%v", c.a, ca, c.b, cb, c.same)
		}
	}
}

func TestParseLen(t *testing.T) {
	if v, ok := parseLen("16px"); !ok || v != 16 {
		t.Errorf("parseLen(16px) = %v,%v", v, ok)
	}
	if v, ok := parseLen("  24  "); !ok || v != 24 {
		t.Errorf("parseLen(24) = %v,%v", v, ok)
	}
	if _, ok := parseLen("auto"); ok {
		t.Error("parseLen(auto) should fail")
	}
}

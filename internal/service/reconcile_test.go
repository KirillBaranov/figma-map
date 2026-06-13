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
	if len(unmeasured) != 1 || unmeasured[0].NodeID != "ghost" {
		t.Fatalf("expected ghost unmeasured, got %+v", unmeasured)
	}
	// A FRAME with no DOM match is actionable (the agent should tag it).
	if !unmeasured[0].Actionable {
		t.Error("FRAME unmeasured should be actionable")
	}
}

func TestClassifyUnmeasured(t *testing.T) {
	// Decorative/image nodes are expected-unmeasured, not actionable.
	for _, typ := range []string{"VECTOR", "RECTANGLE", "LINE", "ELLIPSE"} {
		u := classifyUnmeasured("1", figmaTarget{typ: typ, name: "x"})
		if u.Actionable {
			t.Errorf("%s should not be actionable", typ)
		}
	}
	// Frames/text/instances are actionable (should be tagged).
	for _, typ := range []string{"FRAME", "TEXT", "INSTANCE"} {
		u := classifyUnmeasured("1", figmaTarget{typ: typ, name: "x"})
		if !u.Actionable {
			t.Errorf("%s should be actionable", typ)
		}
	}
}

func TestTier1_ExtendedProperties(t *testing.T) {
	want := map[string]figmaTarget{
		"box": {typ: "FRAME", name: "Box", box: figma.Bounds{Width: 200, Height: 100}, tokens: &Tokens{
			Opacity:      ptr(0.5),
			Stroke:       "#000000",
			StrokeWeight: ptr(2.0),
		}},
	}
	got := map[string]render.DOMElement{
		"box": {FigmaNode: "box",
			Box: render.Box{Width: 150, Height: 100}, // width off by 50, height ok
			Styles: map[string]string{
				"opacity":          "1",              // 1 vs 0.5 → diff
				"border-top-color": "rgb(255, 0, 0)", // wrong color
				"border-top-width": "2px",            // ok
			}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 1 {
		t.Fatalf("want 1 element, got %d", len(byEl))
	}
	props := map[string]FieldDiff{}
	for _, d := range byEl[0].Diffs {
		props[d.Prop] = d
	}
	for _, p := range []string{"opacity", "border-color", "width"} {
		if _, ok := props[p]; !ok {
			t.Errorf("expected diff for %s; got %v", p, props)
		}
	}
	if _, ok := props["height"]; ok {
		t.Error("height matched, should not be flagged")
	}
	if _, ok := props["border-width"]; ok {
		t.Error("border-width matched, should not be flagged")
	}
}

func TestTier1_NoBorderFalsePositive(t *testing.T) {
	// The bridge reports strokeWeight:1 even on borderless nodes; with no stroke
	// color we must NOT flag a border-width difference.
	want := map[string]figmaTarget{
		"b": {typ: "FRAME", name: "B", tokens: &Tokens{StrokeWeight: ptr(1.0)}}, // Stroke == ""
	}
	got := map[string]render.DOMElement{
		"b": {FigmaNode: "b", Styles: map[string]string{"border-top-width": "0px"}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 0 {
		t.Errorf("borderless node should produce no diff, got %+v", byEl)
	}
}

func TestTier1_TextAlignStartEqualsLeft(t *testing.T) {
	want := map[string]figmaTarget{
		"t": {typ: "TEXT", name: "T", tokens: &Tokens{TextAlign: "LEFT", LineHeight: ptr(24.0)}},
	}
	got := map[string]render.DOMElement{
		"t": {FigmaNode: "t", Styles: map[string]string{
			"text-align":  "start", // start == left → no diff
			"line-height": "24px",
		}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 0 {
		t.Errorf("start should equal left, line-height ok; got %+v", byEl)
	}

	// And a real mismatch is caught.
	got["t"].Styles["text-align"] = "center"
	byEl2, _ := tier1Diff(want, got)
	if len(byEl2) != 1 || byEl2[0].Diffs[0].Prop != "text-align" {
		t.Errorf("expected text-align diff, got %+v", byEl2)
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

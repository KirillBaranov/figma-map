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

func TestTier1_TransformSkipsBox(t *testing.T) {
	// A CSS-transformed element: its rect is post-transform, so width/height
	// must not be compared (would be a false diff).
	want := map[string]figmaTarget{
		"x": {typ: "FRAME", name: "X", box: figma.Bounds{Width: 200, Height: 100}, tokens: &Tokens{Fill: "#fff"}},
	}
	got := map[string]render.DOMElement{
		"x": {FigmaNode: "x", Box: render.Box{Width: 400, Height: 200}, Styles: map[string]string{
			"background-color": "rgb(255,255,255)",
			"transform":        "matrix(2, 0, 0, 2, 0, 0)",
		}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 0 {
		t.Errorf("transformed element should not flag width/height, got %+v", byEl)
	}
}

func TestTier1_GeoDiffCatchesTransformCompositionBug(t *testing.T) {
	// The slider case from the strategy doc: a transformed element whose
	// declared box wouldn't tell us anything (see TestTier1_TransformSkipsBox
	// above), but Figma's own post-effects renderBounds is available (geoDiff
	// opted in) and disagrees with the DOM's post-transform box — that's a
	// real transform-origin/composition bug, and it must be reported.
	want := map[string]figmaTarget{
		"x": {typ: "FRAME", name: "X", box: figma.Bounds{Width: 200, Height: 100}, tokens: &Tokens{Fill: "#fff"},
			renderBounds: &figma.Bounds{X: 100, Y: 100, Width: 200, Height: 100}},
	}
	got := map[string]render.DOMElement{
		"x": {FigmaNode: "x", Box: render.Box{X: 114, Y: 97, Width: 200, Height: 100}, Styles: map[string]string{
			"background-color": "rgb(255,255,255)",
			"transform":        "rotate(15deg)",
		}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 1 {
		t.Fatalf("expected a geo-diff finding, got %+v", byEl)
	}
	props := map[string]FieldDiff{}
	for _, d := range byEl[0].Diffs {
		props[d.Prop] = d
	}
	rx, ok := props["render-x"]
	if !ok || rx.Is != "114px" || rx.Should != "100px" {
		t.Errorf("render-x diff missing/wrong: %+v", props)
	}
	if rx.Advisory {
		t.Error("a geo-diff mismatch is a real defect, not advisory")
	}
	if _, ok := props["render-width"]; ok {
		t.Error("width matched (200 == 200), should not be flagged")
	}
}

func TestTier1_GeoDiffSkippedWithoutRenderBounds(t *testing.T) {
	// geoDiff not opted in (renderBounds nil) — must fall back to today's
	// silent skip, not a false diff against the pre-transform declared box.
	want := map[string]figmaTarget{
		"x": {typ: "FRAME", name: "X", box: figma.Bounds{Width: 200, Height: 100}, tokens: &Tokens{Fill: "#fff"}},
	}
	got := map[string]render.DOMElement{
		"x": {FigmaNode: "x", Box: render.Box{X: 999, Y: 999, Width: 400, Height: 200}, Styles: map[string]string{
			"background-color": "rgb(255,255,255)",
			"transform":        "rotate(15deg)",
		}},
	}
	if byEl, _ := tier1Diff(want, got); len(byEl) != 0 {
		t.Errorf("no renderBounds requested → should stay silent, got %+v", byEl)
	}
}

func TestRelativeRenderBounds(t *testing.T) {
	if relativeRenderBounds(nil, 10, 20) != nil {
		t.Error("nil input should return nil")
	}
	got := relativeRenderBounds(&figma.Bounds{X: 110, Y: 120, Width: 50, Height: 60}, 10, 20)
	if got == nil || got.X != 100 || got.Y != 100 || got.Width != 50 || got.Height != 60 {
		t.Errorf("relativeRenderBounds = %+v, want {100,100,50,60}", got)
	}
}

func TestTier1_MissingShadow(t *testing.T) {
	want := map[string]figmaTarget{
		"c": {typ: "FRAME", name: "Card", tokens: &Tokens{Shadow: true}},
	}
	got := map[string]render.DOMElement{
		"c": {FigmaNode: "c", Styles: map[string]string{"box-shadow": "none"}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 1 || byEl[0].Diffs[0].Prop != "box-shadow" {
		t.Errorf("expected missing box-shadow diff, got %+v", byEl)
	}
	// Present shadow → no diff.
	got["c"].Styles["box-shadow"] = "rgba(0,0,0,0.1) 0px 4px 8px 0px"
	if b, _ := tier1Diff(want, got); len(b) != 0 {
		t.Errorf("present shadow should not flag, got %+v", b)
	}
}

func TestTier1_LetterSpacingNormalIsZero(t *testing.T) {
	want := map[string]figmaTarget{
		"t": {typ: "TEXT", name: "T", tokens: &Tokens{LetterSpacing: ptr(0.0)}},
	}
	got := map[string]render.DOMElement{
		"t": {FigmaNode: "t", Styles: map[string]string{"letter-spacing": "normal"}},
	}
	if b, _ := tier1Diff(want, got); len(b) != 0 {
		t.Errorf("letter-spacing normal == 0 should not flag, got %+v", b)
	}
}

func TestTier1_AbsoluteInAutoLayoutIsLinted(t *testing.T) {
	// A DOM position:absolute inside a Figma auto-layout parent, where Figma
	// itself never declared an escape hatch, is a structure-lint issue —
	// independent of whether any pixel or token actually differs.
	want := map[string]figmaTarget{
		"child": {typ: "FRAME", name: "Child", parentAutoLayout: true, tokens: &Tokens{Fill: "#ffffff"}},
	}
	got := map[string]render.DOMElement{
		"child": {FigmaNode: "child", Styles: map[string]string{
			"background-color": "rgb(255, 255, 255)",
			"position":         "absolute",
		}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 1 || len(byEl[0].Diffs) != 1 || byEl[0].Diffs[0].Prop != "position" {
		t.Fatalf("expected a position lint diff, got %+v", byEl)
	}
	if byEl[0].Diffs[0].Advisory {
		t.Error("position lint should be fixable, not advisory")
	}
}

func TestTier1_AbsoluteEscapeHatchNotLinted(t *testing.T) {
	// Figma itself declared this child ABSOLUTE (a legitimate escape hatch
	// inside an auto-layout parent) — DOM position:absolute must not be
	// flagged in that case.
	want := map[string]figmaTarget{
		"child": {typ: "FRAME", name: "Child", parentAutoLayout: true, layoutPositioning: "ABSOLUTE",
			tokens: &Tokens{Fill: "#ffffff"}},
	}
	got := map[string]render.DOMElement{
		"child": {FigmaNode: "child", Styles: map[string]string{
			"background-color": "rgb(255, 255, 255)",
			"position":         "absolute",
		}},
	}
	if byEl, _ := tier1Diff(want, got); len(byEl) != 0 {
		t.Errorf("declared escape hatch should not be linted, got %+v", byEl)
	}
}

func TestTier1_AbsoluteOutsideAutoLayoutNotLinted(t *testing.T) {
	// Not an auto-layout child at all (e.g. top-level or a non-flex parent)
	// — position:absolute here is unremarkable, not a lint finding.
	want := map[string]figmaTarget{
		"child": {typ: "FRAME", name: "Child", tokens: &Tokens{Fill: "#ffffff"}},
	}
	got := map[string]render.DOMElement{
		"child": {FigmaNode: "child", Styles: map[string]string{
			"background-color": "rgb(255, 255, 255)",
			"position":         "absolute",
		}},
	}
	if byEl, _ := tier1Diff(want, got); len(byEl) != 0 {
		t.Errorf("non-auto-layout child should not be linted, got %+v", byEl)
	}
}

func TestTier1_ColorDiffCarriesVariableHint(t *testing.T) {
	// When Figma's color is bound to a Variable, a color mismatch's "should"
	// value carries the binding name as a hint — not a separate assertion,
	// the match/no-match criterion is unchanged.
	want := map[string]figmaTarget{
		"btn": {typ: "FRAME", name: "Button", tokens: &Tokens{
			Fill: "#18181b", FillVariable: "Colors/Text/Primary",
		}},
	}
	got := map[string]render.DOMElement{
		"btn": {FigmaNode: "btn", Styles: map[string]string{"background-color": "rgb(31, 41, 55)"}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 1 || len(byEl[0].Diffs) != 1 {
		t.Fatalf("expected 1 color diff, got %+v", byEl)
	}
	should := byEl[0].Diffs[0].Should
	if should != "#18181b (Figma Variable: Colors/Text/Primary)" {
		t.Errorf("should = %q, want the hex plus the variable hint", should)
	}
}

func TestTier1_ColorDiffNoHintWhenUnbound(t *testing.T) {
	// No Variable bound → "should" is unchanged, no hint appended.
	want := map[string]figmaTarget{
		"btn": {typ: "FRAME", name: "Button", tokens: &Tokens{Fill: "#18181b"}},
	}
	got := map[string]render.DOMElement{
		"btn": {FigmaNode: "btn", Styles: map[string]string{"background-color": "rgb(31, 41, 55)"}},
	}
	byEl, _ := tier1Diff(want, got)
	if len(byEl) != 1 || byEl[0].Diffs[0].Should != "#18181b" {
		t.Fatalf("expected plain hex should with no hint, got %+v", byEl)
	}
}

func TestIssuesFromElements(t *testing.T) {
	byElement := []ElementDiff{
		{NodeID: "a", Name: "A", Diffs: []FieldDiff{
			{Prop: "gap", Is: "16px", Should: "24px"},
			{Prop: "width", Is: "100px", Should: "120px", Advisory: true},
		}},
		{NodeID: "b", Name: "B", Diffs: []FieldDiff{
			{Prop: "color", Is: "red", Should: "blue"},
		}},
	}
	issues := issuesFromElements(byElement, []string{"b"}) // "b" matched spatially, not by tag

	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d: %+v", len(issues), issues)
	}
	byProp := map[string]Issue{}
	for _, i := range issues {
		byProp[i.NodeID+"."+i.Property] = i
	}

	gap := byProp["a.gap"]
	if gap.Severity != "major" || gap.Source != "structured" {
		t.Errorf("gap issue = %+v, want major/structured", gap)
	}
	if gap.Confidence != 1.0 || gap.DOMSelector != `[data-figma-node="a"]` {
		t.Errorf("tag-matched issue should have full confidence + a selector, got %+v", gap)
	}

	width := byProp["a.width"]
	if width.Severity != "advisory" {
		t.Errorf("advisory diff should map to advisory severity, got %+v", width)
	}

	color := byProp["b.color"]
	if color.Confidence != 0.6 || color.DOMSelector != "" {
		t.Errorf("spatially-aligned issue should have lower confidence + no selector, got %+v", color)
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

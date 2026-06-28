package figma

import (
	"encoding/json"
	"testing"
)

// fixture mirrors the bridge serializer output for a frame + a text child,
// including the number|"mixed" union the decoder must tolerate.
const styleFixture = `{
  "id": "1:1", "name": "Card", "type": "FRAME",
  "bounds": {"x":0,"y":0,"width":350,"height":200},
  "styles": {
    "opacity": 1,
    "blendMode": "MULTIPLY",
    "fills": [{"type":"SOLID","color":"#ffffff","opacity":1,"variable":"Color/Surface/White"}],
    "cornerRadius": 8,
    "cornerSmoothing": 0.6,
    "strokeWeight": "mixed",
    "dashPattern": [4, 2],
    "autoLayout": {"direction":"VERTICAL","gap":16,"primaryAxisAlign":"MIN","counterAxisAlign":"CENTER"},
    "padding": {"top":32,"right":32,"bottom":64,"left":32},
    "clipsContent": true,
    "constraints": {"horizontal":"MAX","vertical":"MIN"},
    "boundVariables": {"itemSpacing":"Spacing/sm","topLeftRadius":"Radius/md"}
  },
  "children": [
    {"id":"1:2","name":"Title","type":"TEXT","characters":"Hello",
     "bounds":{"x":32,"y":32,"width":286,"height":24},
     "styles":{
       "fills":[{"type":"SOLID","color":"#18181b","opacity":1}],
       "fontSize":24,"fontFamily":"Inter","fontWeight":600,
       "lineHeight":{"unit":"PIXELS","value":32},
       "textAlignHorizontal":"LEFT"
     }}
  ]
}`

func TestStyleDecode(t *testing.T) {
	var n Node
	if err := json.Unmarshal([]byte(styleFixture), &n); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if n.Styles == nil {
		t.Fatal("frame styles not decoded")
	}
	// Color
	if len(n.Styles.Fills.Paints) != 1 || n.Styles.Fills.Paints[0].Color != "#ffffff" {
		t.Errorf("fills = %+v", n.Styles.Fills.Paints)
	}
	// Number token
	if !n.Styles.CornerRadius.Set || n.Styles.CornerRadius.Value != 8 {
		t.Errorf("cornerRadius = %+v", n.Styles.CornerRadius)
	}
	// number|"mixed" union must not break decoding
	if !n.Styles.StrokeWeight.Mixed {
		t.Errorf("strokeWeight should be mixed, got %+v", n.Styles.StrokeWeight)
	}
	// Auto-layout
	if n.Styles.AutoLayout == nil || n.Styles.AutoLayout.Direction != "VERTICAL" || n.Styles.AutoLayout.Gap != 16 {
		t.Errorf("autoLayout = %+v", n.Styles.AutoLayout)
	}
	// Padding
	if n.Styles.Padding == nil || n.Styles.Padding.Bottom != 64 {
		t.Errorf("padding = %+v", n.Styles.Padding)
	}
	// Previously-dropped fields (Phase 0): now decoded instead of discarded.
	if n.Styles.BlendMode != "MULTIPLY" {
		t.Errorf("blendMode = %q", n.Styles.BlendMode)
	}
	if n.Styles.CornerSmoothing == nil || *n.Styles.CornerSmoothing != 0.6 {
		t.Errorf("cornerSmoothing = %+v", n.Styles.CornerSmoothing)
	}
	if len(n.Styles.DashPattern) != 2 || n.Styles.DashPattern[0] != 4 || n.Styles.DashPattern[1] != 2 {
		t.Errorf("dashPattern = %+v", n.Styles.DashPattern)
	}
	if n.Styles.ClipsContent == nil || !*n.Styles.ClipsContent {
		t.Errorf("clipsContent = %+v", n.Styles.ClipsContent)
	}
	if n.Styles.Constraints == nil || n.Styles.Constraints.Horizontal != "MAX" || n.Styles.Constraints.Vertical != "MIN" {
		t.Errorf("constraints = %+v", n.Styles.Constraints)
	}
	// Phase 2: per-paint and per-field variable bindings.
	if len(n.Styles.Fills.Paints) != 1 || n.Styles.Fills.Paints[0].Variable != "Color/Surface/White" {
		t.Errorf("fill variable = %+v", n.Styles.Fills.Paints)
	}
	if got := FirstSolidVariable(n.Styles.Fills.Paints); got != "Color/Surface/White" {
		t.Errorf("FirstSolidVariable = %q", got)
	}
	if n.Styles.BoundVariables["itemSpacing"] != "Spacing/sm" || n.Styles.BoundVariables["topLeftRadius"] != "Radius/md" {
		t.Errorf("boundVariables = %+v", n.Styles.BoundVariables)
	}

	// Text child typography
	if len(n.Children) != 1 {
		t.Fatalf("want 1 child, got %d", len(n.Children))
	}
	txt := n.Children[0].Styles
	if txt == nil || !txt.FontSize.Set || txt.FontSize.Value != 24 {
		t.Errorf("text fontSize = %+v", txt)
	}
	if txt.FontFamily != "Inter" || !txt.FontWeight.Set || txt.FontWeight.Value != 600 {
		t.Errorf("text font = %+v", txt)
	}
	if txt.LineHeight == nil || txt.LineHeight.Unit != "PIXELS" || txt.LineHeight.Value != 32 {
		t.Errorf("lineHeight = %+v", txt.LineHeight)
	}
}

// TestGridLayoutDecode covers Phase 3: a GRID auto-layout frame and a
// gridPosition-bearing child decode without dropping the grid-specific
// fields (the non-grid styleFixture above never exercises GRID/gridPosition).
const gridFixture = `{
  "id": "1:1", "name": "Grid", "type": "FRAME",
  "bounds": {"x":0,"y":0,"width":400,"height":300},
  "styles": {
    "autoLayout": {
      "direction":"GRID",
      "gap":0,
      "gridRowSizes":[{"type":"FIXED","value":120},{"type":"FLEX","value":1}],
      "gridColumnSizes":[{"type":"HUG"}],
      "gridRowGap":8,
      "gridColumnGap":16
    }
  },
  "children": [
    {"id":"1:2","name":"Cell","type":"FRAME",
     "bounds":{"x":0,"y":0,"width":100,"height":100},
     "gridPosition":{"rowIndex":1,"columnIndex":0,"rowSpan":1,"columnSpan":1}}
  ]
}`

func TestGridLayoutDecode(t *testing.T) {
	var n Node
	if err := json.Unmarshal([]byte(gridFixture), &n); err != nil {
		t.Fatalf("decode: %v", err)
	}
	al := n.Styles.AutoLayout
	if al == nil || al.Direction != "GRID" {
		t.Fatalf("autoLayout = %+v", al)
	}
	if len(al.GridRowSizes) != 2 || al.GridRowSizes[0].Type != "FIXED" || *al.GridRowSizes[0].Value != 120 {
		t.Errorf("gridRowSizes = %+v", al.GridRowSizes)
	}
	if len(al.GridColumnSizes) != 1 || al.GridColumnSizes[0].Type != "HUG" {
		t.Errorf("gridColumnSizes = %+v", al.GridColumnSizes)
	}
	if al.GridRowGap != 8 || al.GridColumnGap != 16 {
		t.Errorf("grid gaps = %+v", al)
	}

	if len(n.Children) != 1 {
		t.Fatalf("want 1 child, got %d", len(n.Children))
	}
	gp := n.Children[0].GridPosition
	if gp == nil || gp.RowIndex != 1 || gp.ColumnIndex != 0 || gp.RowSpan != 1 || gp.ColumnSpan != 1 {
		t.Errorf("gridPosition = %+v", gp)
	}
}

// TestReactionsDecode covers Phase 4: a node's prototyping reactions
// (trigger + transition timing) decode without dropping fields.
const reactionsFixture = `{
  "id": "1:1", "name": "Button", "type": "INSTANCE",
  "bounds": {"x":0,"y":0,"width":100,"height":40},
  "reactions": [{"trigger":"ON_HOVER","transitionType":"SMART_ANIMATE","easing":"EASE_OUT","duration":0.2}]
}`

func TestReactionsDecode(t *testing.T) {
	var n Node
	if err := json.Unmarshal([]byte(reactionsFixture), &n); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(n.Reactions) != 1 {
		t.Fatalf("want 1 reaction, got %d", len(n.Reactions))
	}
	r := n.Reactions[0]
	if r.Trigger != "ON_HOVER" || r.TransitionType != "SMART_ANIMATE" || r.Easing != "EASE_OUT" {
		t.Errorf("reaction = %+v", r)
	}
	if r.Duration == nil || *r.Duration != 0.2 {
		t.Errorf("duration = %+v", r.Duration)
	}
}

func TestMaybeNumRoundTrip(t *testing.T) {
	cases := map[string]struct {
		mixed bool
		set   bool
		val   float64
	}{
		"8":       {false, true, 8},
		`"mixed"`: {true, true, 0},
		"null":    {false, false, 0},
	}
	for in, want := range cases {
		var m MaybeNum
		if err := json.Unmarshal([]byte(in), &m); err != nil {
			t.Fatalf("unmarshal %s: %v", in, err)
		}
		if m.Mixed != want.mixed || m.Set != want.set || m.Value != want.val {
			t.Errorf("%s → %+v, want %+v", in, m, want)
		}
	}
}

// TestCSSColorPrefersCodeSyntax verifies CSSColor emits var(--token) when
// the paint's bound variable has a designer-set WEB CodeSyntax, falls back
// to the literal hex otherwise, and never substitutes CodeSyntax when the
// paint carries fractional opacity (var() can't absorb the extracted alpha).
func TestCSSColorPrefersCodeSyntax(t *testing.T) {
	cases := []struct {
		name string
		p    Paint
		want string
	}{
		{"opaque with code syntax", Paint{Type: "SOLID", Color: "#18181b", Opacity: 1, CodeSyntax: "--color-brand-primary"}, "var(--color-brand-primary)"},
		{"opaque without code syntax", Paint{Type: "SOLID", Color: "#18181b", Opacity: 1}, "#18181b"},
		{"code syntax missing leading --", Paint{Type: "SOLID", Color: "#18181b", Opacity: 1, CodeSyntax: "color-textIcon-default"}, "var(--color-textIcon-default)"},
		{"fractional opacity ignores code syntax", Paint{Type: "SOLID", Color: "#18181b", Opacity: 0.1, CodeSyntax: "--color-brand-primary"}, "rgba(24, 24, 27, 0.1)"},
		{"not solid", Paint{Type: "IMAGE", CodeSyntax: "--color-brand-primary"}, ""},
	}
	for _, c := range cases {
		if got := c.p.CSSColor(); got != c.want {
			t.Errorf("%s: CSSColor() = %q, want %q", c.name, got, c.want)
		}
	}
}

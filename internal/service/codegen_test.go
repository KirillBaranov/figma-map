package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// TestBuildFrameStyle_ClipsContent covers the Phase 0 fix: clipsContent=true
// now becomes overflow:hidden; false/absent stays exactly as before (no
// overflow declared at all).
func TestBuildFrameStyle_ClipsContent(t *testing.T) {
	clips := true
	p := buildFrameStyle(&figma.Style{ClipsContent: &clips}, figma.Bounds{Width: 100, Height: 100}, false, figma.Bounds{})
	if got := p.renderJSX(); !strings.Contains(got, "overflow: 'hidden'") {
		t.Errorf("clipsContent=true should add overflow:hidden, got %s", got)
	}

	noClip := false
	p2 := buildFrameStyle(&figma.Style{ClipsContent: &noClip}, figma.Bounds{Width: 100, Height: 100}, false, figma.Bounds{})
	if got := p2.renderJSX(); strings.Contains(got, "overflow") {
		t.Errorf("clipsContent=false should not add overflow, got %s", got)
	}

	p3 := buildFrameStyle(&figma.Style{}, figma.Bounds{Width: 100, Height: 100}, false, figma.Bounds{})
	if got := p3.renderJSX(); strings.Contains(got, "overflow") {
		t.Errorf("absent clipsContent should not add overflow, got %s", got)
	}
}

// TestBuildFrameStyle_Rotation verifies the CSS rotate() sign is negated
// relative to Figma's own rotation value — Figma's is positive=counter-
// clockwise, CSS rotate() is positive=clockwise (Figma's own Dev Mode panel
// negates for this exact reason), so passing the raw value through verbatim
// spins every rotated element the wrong way.
func TestBuildFrameStyle_Rotation(t *testing.T) {
	rotation := 30.0
	p := buildFrameStyle(&figma.Style{Rotation: &rotation}, figma.Bounds{Width: 100, Height: 100}, false, figma.Bounds{})
	if got := p.renderJSX(); !strings.Contains(got, "rotate(-30deg)") {
		t.Errorf("Figma rotation 30 should become CSS rotate(-30deg), got %s", got)
	}
}

// TestBuildFrameStyle_Constraints covers the Phase 0 fix: a MAX constraint on
// an absolutely-positioned child pins it to the parent's far edge
// (right/bottom) instead of always emitting left/top.
func TestBuildFrameStyle_Constraints(t *testing.T) {
	parent := figma.Bounds{Width: 400, Height: 300}
	bounds := figma.Bounds{X: 350, Y: 270, Width: 30, Height: 20} // 20px from right, 10px from bottom

	style := &figma.Style{Constraints: &figma.Constraints{Horizontal: "MAX", Vertical: "MAX"}}
	p := buildFrameStyle(style, bounds, true, parent)
	got := p.renderJSX()
	if !strings.Contains(got, "right: '20px'") {
		t.Errorf("MAX horizontal constraint should add right:20px, got %s", got)
	}
	if !strings.Contains(got, "bottom: '10px'") {
		t.Errorf("MAX vertical constraint should add bottom:10px, got %s", got)
	}
	if strings.Contains(got, "left:") || strings.Contains(got, "top:") {
		t.Errorf("MAX-constrained node should not also get left/top, got %s", got)
	}

	// No constraints (or non-MAX) falls back to the original left/top behavior.
	p2 := buildFrameStyle(&figma.Style{}, bounds, true, parent)
	got2 := p2.renderJSX()
	if !strings.Contains(got2, "left: '350px'") || !strings.Contains(got2, "top: '270px'") {
		t.Errorf("no constraints should fall back to left/top, got %s", got2)
	}
}

// TestBuildFrameStyle_Grid covers Phase 3: a GRID auto-layout frame becomes
// display:grid with its explicit row/column tracks — ground truth from
// Figma's own grid setup, not an inferred structure.
func TestBuildFrameStyle_Grid(t *testing.T) {
	fixed := 120.0
	style := &figma.Style{AutoLayout: &figma.AutoLayout{
		Direction:       "GRID",
		GridRowSizes:    []figma.GridTrack{{Type: "FIXED", Value: &fixed}, {Type: "FLEX"}},
		GridColumnSizes: []figma.GridTrack{{Type: "HUG"}},
		GridRowGap:      8,
		GridColumnGap:   16,
	}}
	p := buildFrameStyle(style, figma.Bounds{Width: 400, Height: 300}, false, figma.Bounds{})
	got := p.renderJSX()
	if !strings.Contains(got, "display: 'grid'") {
		t.Errorf("GRID direction should set display:grid, got %s", got)
	}
	if !strings.Contains(got, "gridTemplateRows: '120px 1fr'") {
		t.Errorf("gridTemplateRows = %s", got)
	}
	if !strings.Contains(got, "gridTemplateColumns: 'max-content'") {
		t.Errorf("gridTemplateColumns = %s", got)
	}
	if !strings.Contains(got, "rowGap: '8px'") || !strings.Contains(got, "columnGap: '16px'") {
		t.Errorf("row/column gap = %s", got)
	}
}

// TestGridPlacement covers a grid child's explicit row/column placement.
func TestGridPlacement(t *testing.T) {
	p := &styleProps{}
	addGridPlacement(p, &figma.GridPosition{RowIndex: 1, ColumnIndex: 2, RowSpan: 1, ColumnSpan: 2})
	got := p.renderJSX()
	if !strings.Contains(got, "gridRow: '2 / span 1'") {
		t.Errorf("gridRow = %s", got)
	}
	if !strings.Contains(got, "gridColumn: '3 / span 2'") {
		t.Errorf("gridColumn = %s", got)
	}

	// A shape with a grid position must not be position:absolute, or
	// gridRow/gridColumn placement would have no effect.
	shapeProps := buildShapeStyle(nil, figma.Bounds{Width: 10, Height: 10}, &figma.GridPosition{})
	if strings.Contains(shapeProps.renderJSX(), "position") {
		t.Errorf("grid-positioned shape should not be position:absolute, got %s", shapeProps.renderJSX())
	}
}

// TestBuildFrameStyle_StrokeWeights covers Phase 5: per-side stroke weights
// become individual border-side declarations instead of a single `border`.
func TestBuildFrameStyle_StrokeWeights(t *testing.T) {
	style := &figma.Style{
		Strokes:       figma.MaybePaints{Paints: []figma.Paint{{Type: "SOLID", Color: "#000000"}}},
		StrokeWeights: &figma.Sides{Top: 2, Right: 1, Bottom: 1, Left: 1},
	}
	p := buildFrameStyle(style, figma.Bounds{Width: 10, Height: 10}, false, figma.Bounds{})
	got := p.renderJSX()
	if !strings.Contains(got, "borderTop: '2px solid #000000'") {
		t.Errorf("borderTop = %s", got)
	}
	if strings.Contains(got, "border:") {
		t.Errorf("should not also emit uniform border, got %s", got)
	}
}

// TestFrame_LayoutPositioningAbsolute covers Phase 5: a child with
// layoutPositioning=ABSOLUTE stays absolutely positioned even though its
// parent is auto-layout.
func TestFrame_LayoutPositioningAbsolute(t *testing.T) {
	parent := &figma.Node{
		ID: "1", Type: "FRAME", Bounds: figma.Bounds{Width: 200, Height: 100},
		Styles: &figma.Style{AutoLayout: &figma.AutoLayout{Direction: "HORIZONTAL"}},
		Children: []figma.Node{
			{ID: "2", Type: "FRAME", Bounds: figma.Bounds{X: 5, Y: 5, Width: 10, Height: 10},
				Styles: &figma.Style{LayoutPositioning: "ABSOLUTE"}},
		},
	}
	g := &codeGen{}
	out := g.frame(parent, 0, false, figma.Bounds{})
	if !strings.Contains(out, "position: 'absolute'") {
		t.Errorf("ABSOLUTE child should stay absolutely positioned, got:\n%s", out)
	}
}

// TestVector_ExportsSVG covers Phase 6: a VECTOR node exports to an SVG file
// and becomes an <img>, instead of being dropped as a comment.
func TestVector_ExportsSVG(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	fake := &fakeSource{png: []byte("<svg></svg>")}
	g := &codeGen{ctx: context.Background(), src: fake, fileKey: "k"}
	n := &figma.Node{ID: "1:9", Type: "VECTOR", Name: "Icon", Bounds: figma.Bounds{Width: 16, Height: 16}}

	out := g.vector(n, 0)
	if !strings.Contains(out, "<img src=") {
		t.Errorf("expected an <img>, got %s", out)
	}
	if strings.Contains(out, "/* SVG:") {
		t.Errorf("should not fall back to a comment on success, got %s", out)
	}

	// HTML-preview mode never exports — stays network-free.
	g.html = true
	out2 := g.vector(n, 0)
	if !strings.Contains(out2, "<!-- SVG: Icon -->") {
		t.Errorf("html mode should fall back to a comment, got %s", out2)
	}

	// A failed export (empty bytes from the source) shouldn't fail codegen —
	// it falls back to the comment, same as before this feature existed.
	failGen := &codeGen{ctx: context.Background(), src: &fakeSource{}, fileKey: "k"}
	out3 := failGen.vector(n, 0)
	if !strings.Contains(out3, "{/* SVG: Icon */}") {
		t.Errorf("failed export should fall back to a comment, got %s", out3)
	}
}

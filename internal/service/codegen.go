package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/codegen"
	"github.com/kirillbaranov/figma-map/internal/codegen/ir"
	_ "github.com/kirillbaranov/figma-map/internal/codegen/targets/htmlrender" // registers the "html" codegen target
	_ "github.com/kirillbaranov/figma-map/internal/codegen/targets/jsx"        // registers the "jsx" codegen target
	"github.com/kirillbaranov/figma-map/internal/figma"
)

// CodegenResult is the file generated for a Figma frame, in whichever target
// the caller selected (jsx by default).
type CodegenResult struct {
	NodeID    string `json:"nodeId"`
	Name      string `json:"name"`
	Component string `json:"component"` // exported function name
	Target    string `json:"target"`    // renderer used, e.g. "jsx"
	Code      string `json:"code"`

	// SchemaVersion identifies the shape of this result for external
	// consumers (CLI/MCP callers). It only changes if a future edit alters
	// what these fields mean, not when new optional fields are added.
	SchemaVersion int `json:"schemaVersion"`
}

// Codegen walks the full Figma node tree, builds a target-neutral ir.Node
// tree, and renders it with the selected target (default "jsx" — see
// resolveTarget). Every FRAME, GROUP, TEXT, RECTANGLE and INSTANCE node is
// translated with inline styles derived from Figma auto-layout, fills, and
// typography. INSTANCE nodes matched against the binding are emitted as
// UIKit components.
func (s *Service) Codegen(ctx context.Context, fileKey, nodeID, bindingPath, target string) (CodegenResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return CodegenResult{}, err
	}

	node, err := s.src.Node(ctx, key, nodeID)
	if err != nil {
		return CodegenResult{}, err
	}

	b, _ := binding.Load(bindingPath) // non-fatal: proceed without UIKit mapping

	target = resolveTarget(target, s.cfg.Codegen.Target)
	renderer, ok := codegen.Get(target)
	if !ok {
		return CodegenResult{}, fmt.Errorf("unknown codegen target %q (supported: %s)", target, strings.Join(codegen.Registered(), ", "))
	}

	// node.Bounds.X/Y are absolute canvas coordinates. The generated component
	// is meant to be embeddable on its own, not glued to its original canvas
	// position, so the root renders at its own origin; descendants keep their
	// parent-relative bounds untouched.
	root := *node
	root.Bounds.X, root.Bounds.Y = 0, 0

	gen := &codeGen{b: b, ctx: ctx, src: s.src, fileKey: key}
	tree := gen.node(&root, false, root.Bounds)

	compName := pascalCase(node.Name)
	code := renderer.RenderFile(tree, gen.used, compName)

	return CodegenResult{
		NodeID: node.ID, Name: node.Name, Component: compName,
		Target: target, Code: code, SchemaVersion: 1,
	}, nil
}

// resolveTarget picks the effective codegen target: an explicit argument
// wins, then the project's figma-map.yaml default, then the hardcoded "jsx"
// fallback.
func resolveTarget(explicit, configDefault string) string {
	if explicit != "" {
		return explicit
	}
	if configDefault != "" {
		return configDefault
	}
	return "jsx"
}

// codeGen holds per-run state while walking the Figma tree into a
// target-neutral ir.Node tree (see internal/codegen/ir). Serialization to a
// specific target's source text happens afterward, via codegen.Renderer.
type codeGen struct {
	b    binding.Binding
	used map[string]string // UIKit symbol → import path

	// ctx/src/fileKey are only set for the real Codegen path (not the
	// network-free HTML preview build in preview.go) — used to export vector
	// nodes to SVG files instead of dropping them as comments.
	ctx     context.Context
	src     figma.Source
	fileKey string
}

func (g *codeGen) addImport(sym, path string) {
	if g.used == nil {
		g.used = map[string]string{}
	}
	g.used[sym] = path
}

// node builds the ir.Node for one Figma node and its entire subtree.
// parentHasAutoLayout=true means the parent is a flex container — children are
// in flow and must not be absolutely positioned. false means the parent uses
// position:relative, so children need position:absolute + left/top (or
// right/bottom, see buildFrameStyle's Constraints handling) to place
// themselves correctly. parentBounds is the immediate parent's Figma bounds,
// needed to compute a right/bottom offset from a MAX constraint.
func (g *codeGen) node(n *figma.Node, parentHasAutoLayout bool, parentBounds figma.Bounds) *ir.Node {
	if n.Styles != nil && n.Styles.Visible != nil && !*n.Styles.Visible {
		return &ir.Node{Kind: ir.Comment, Text: fmt.Sprintf("hidden: %s", n.Name)}
	}

	absolute := !parentHasAutoLayout

	switch n.Type {
	case "TEXT":
		return g.text(n, absolute, parentBounds)
	case "INSTANCE":
		if comp, ok := g.findComp(n.Name); ok {
			return g.component(n, comp)
		}
		return g.frame(n, absolute, parentBounds)
	case "VECTOR", "BOOLEAN_OPERATION", "STAR", "POLYGON":
		return g.vector(n)
	default:
		if len(n.Children) > 0 || n.Type == "GROUP" || n.Type == "FRAME" ||
			n.Type == "COMPONENT" || n.Type == "COMPONENT_SET" {
			return g.frame(n, absolute, parentBounds)
		}
		return g.shape(n)
	}
}

func (g *codeGen) text(n *figma.Node, absolute bool, parentBounds figma.Bounds) *ir.Node {
	p := buildTextStyle(n.Styles, n.Bounds, absolute, parentBounds)
	addGridPlacement(p, n.GridPosition)
	text := strings.ReplaceAll(n.Characters, "\n", " ")

	tag := "span"
	if n.Styles != nil && n.Styles.FontSize.Set && n.Styles.FontSize.Value >= 24 {
		tag = "p"
	}

	return &ir.Node{
		Kind:     ir.Element,
		Tag:      tag,
		Style:    p,
		Children: []*ir.Node{{Kind: ir.TextLeaf, Text: text}},
	}
}

func (g *codeGen) frame(n *figma.Node, absolute bool, parentBounds figma.Bounds) *ir.Node {
	p := buildFrameStyle(n.Styles, n.Bounds, absolute, parentBounds)
	addGridPlacement(p, n.GridPosition)

	// Children are in flow when THIS frame has autolayout.
	childrenInFlow := n.Styles != nil && n.Styles.AutoLayout != nil

	if len(n.Children) == 0 {
		return &ir.Node{Kind: ir.Element, Tag: "div", Style: p}
	}

	children := make([]*ir.Node, 0, len(n.Children)*2)
	for i := range n.Children {
		child := &n.Children[i]
		if child.Type != "TEXT" {
			children = append(children, &ir.Node{Kind: ir.Comment, Text: child.Name})
		}
		// A child can opt out of the parent's auto-layout flow via Figma's
		// "ABSOLUTE" escape hatch even when the parent itself is auto-layout.
		notAbsoluteChild := child.Styles == nil || child.Styles.LayoutPositioning != "ABSOLUTE"
		childInFlow := childrenInFlow && notAbsoluteChild
		children = append(children, g.node(child, childInFlow, n.Bounds))
	}

	return &ir.Node{Kind: ir.Element, Tag: "div", Style: p, Children: children}
}

func (g *codeGen) shape(n *figma.Node) *ir.Node {
	p := buildShapeStyle(n.Styles, n.Bounds, n.GridPosition)
	addGridPlacement(p, n.GridPosition)
	return &ir.Node{Kind: ir.Element, Tag: "div", Style: p}
}

// vector builds the ir.Node for a VECTOR/BOOLEAN_OPERATION/STAR/POLYGON leaf
// node. When the tree-walk has network access (the real Codegen path — the
// network-free HTML preview build never sets src), it exports the node to an
// SVG file and emits an image element. Any export failure (or no network
// access) falls back to a comment — a failed icon export shouldn't fail the
// whole codegen.
func (g *codeGen) vector(n *figma.Node) *ir.Node {
	if g.src != nil {
		if path, ok := g.exportVectorSVG(n); ok {
			p := buildShapeStyle(n.Styles, n.Bounds, n.GridPosition)
			addGridPlacement(p, n.GridPosition)
			return &ir.Node{Kind: ir.Element, Tag: "img", Src: path, Style: p, SelfClose: true}
		}
	}
	return &ir.Node{Kind: ir.Comment, Text: fmt.Sprintf("SVG: %s", n.Name)}
}

// exportVectorSVG exports n as an SVG file under the default output
// convention (see defaultOutPath) and returns its path.
func (g *codeGen) exportVectorSVG(n *figma.Node) (string, bool) {
	data, err := g.src.Screenshot(g.ctx, g.fileKey, n.ID, figma.ScreenshotOpts{Format: "SVG"})
	if err != nil || len(data) == 0 {
		return "", false
	}
	path := defaultOutPath(n.ID, "icon", ".svg")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", false
	}
	return path, true
}

func (g *codeGen) component(n *figma.Node, comp binding.Component) *ir.Node {
	g.addImport(comp.Symbol, comp.Import)

	// Prefer this instance's actual Figma componentProps (ground truth —
	// see resolvePropsFromFigma) over the binding's bare default; only props
	// that don't resolve from Figma fall back to their default placeholder.
	resolved, _ := resolvePropsFromFigma(n.ComponentProps, comp.Props)
	props := map[string]string{}
	for name, prop := range comp.Props {
		if v, ok := resolved[name]; ok {
			props[name] = v
		} else if def := prop.Default(); def != "" {
			props[name] = def
		}
	}

	return &ir.Node{
		Kind: ir.Component, Tag: comp.Symbol,
		Import: &ir.Import{Symbol: comp.Symbol, Path: comp.Import},
		Props:  props, Text: n.FirstText(),
	}
}

// findComp looks up a binding match for an INSTANCE by name containment
// (case-insensitive). Component name keys and Symbol are both tried.
func (g *codeGen) findComp(nodeName string) (binding.Component, bool) {
	lower := strings.ToLower(nodeName)
	for name, comp := range g.b.Components {
		if strings.Contains(lower, strings.ToLower(name)) ||
			strings.Contains(lower, strings.ToLower(comp.Symbol)) {
			return comp, true
		}
	}
	return binding.Component{}, false
}

// --- style builders ---

func buildFrameStyle(s *figma.Style, bounds figma.Bounds, absolute bool, parentBounds figma.Bounds) *ir.Style {
	p := &ir.Style{}

	// Position and placement.
	if absolute {
		p.Add("position", q("absolute"))
		addConstrainedOffsets(p, s, bounds, parentBounds)
	}

	if s == nil {
		if !absolute {
			p.Add("position", q("relative"))
		}
		p.Add("width", px(bounds.Width))
		p.Add("height", px(bounds.Height))
		return p
	}

	if al := s.AutoLayout; al != nil && al.Direction == "GRID" {
		// Figma's native CSS-grid-like auto-layout — the designer's own
		// explicit row/column setup, surfaced as-is, never inferred from
		// freeform positioning.
		p.Add("display", q("grid"))
		if len(al.GridRowSizes) > 0 {
			p.Add("gridTemplateRows", q(cssTrackList(al.GridRowSizes)))
		}
		if len(al.GridColumnSizes) > 0 {
			p.Add("gridTemplateColumns", q(cssTrackList(al.GridColumnSizes)))
		}
		if al.GridRowGap == al.GridColumnGap {
			if al.GridRowGap > 0 {
				p.Add("gap", px(al.GridRowGap))
			}
		} else {
			if al.GridRowGap > 0 {
				p.Add("rowGap", px(al.GridRowGap))
			}
			if al.GridColumnGap > 0 {
				p.Add("columnGap", px(al.GridColumnGap))
			}
		}
	} else if al := s.AutoLayout; al != nil {
		p.Add("display", q("flex"))
		switch al.Direction {
		case "HORIZONTAL":
			p.Add("flexDirection", q("row"))
		case "VERTICAL":
			p.Add("flexDirection", q("column"))
		}
		if al.Gap > 0 {
			p.Add("gap", px(al.Gap))
		}
		switch al.PrimaryAxisAlign {
		case "CENTER":
			p.Add("justifyContent", q("center"))
		case "MAX":
			p.Add("justifyContent", q("flex-end"))
		case "SPACE_BETWEEN":
			p.Add("justifyContent", q("space-between"))
		}
		switch al.CounterAxisAlign {
		case "CENTER":
			p.Add("alignItems", q("center"))
		case "MAX":
			p.Add("alignItems", q("flex-end"))
		}
		if al.Wrap == "WRAP" {
			p.Add("flexWrap", q("wrap"))
		}

		// FIXED axes carry an explicit size in Figma; AUTO axes hug content
		// and must stay unset so the flex box can shrink/grow naturally.
		primaryFixed := al.PrimaryAxisSizing == "FIXED"
		counterFixed := al.CounterAxisSizing == "FIXED"
		widthFixed, heightFixed := counterFixed, primaryFixed
		if al.Direction == "HORIZONTAL" {
			widthFixed, heightFixed = primaryFixed, counterFixed
		}
		if widthFixed && bounds.Width > 0 {
			p.Add("width", px(bounds.Width))
		}
		if heightFixed && bounds.Height > 0 {
			p.Add("height", px(bounds.Height))
		}
	} else {
		if !absolute {
			p.Add("position", q("relative"))
		}
		if bounds.Width > 0 {
			p.Add("width", px(bounds.Width))
		}
		if bounds.Height > 0 {
			p.Add("height", px(bounds.Height))
		}
	}

	if pad := s.Padding; pad != nil {
		switch {
		case pad.Top == pad.Right && pad.Right == pad.Bottom && pad.Bottom == pad.Left:
			p.Add("padding", px(pad.Top))
		case pad.Top == pad.Bottom && pad.Left == pad.Right:
			p.Add("padding", fmt.Sprintf("'%gpx %gpx'", pad.Top, pad.Left))
		default:
			p.Add("paddingTop", px(pad.Top))
			p.Add("paddingRight", px(pad.Right))
			p.Add("paddingBottom", px(pad.Bottom))
			p.Add("paddingLeft", px(pad.Left))
		}
	}

	if c := figma.FirstSolidCSS(s.Fills.Paints); c != "" {
		p.Add("background", q(c))
	}

	if s.CornerRadius.Set && !s.CornerRadius.Mixed && s.CornerRadius.Value > 0 {
		if s.CornerRadius.Value >= 9999 {
			p.Add("borderRadius", q("9999px"))
		} else {
			p.Add("borderRadius", px(s.CornerRadius.Value))
		}
	}

	if c := figma.FirstSolidCSS(s.Strokes.Paints); c != "" && len(s.Strokes.Paints) > 0 {
		switch {
		case s.StrokeWeights != nil:
			// Per-side weights differ — a single `border` shorthand can't
			// express that.
			p.Add("borderTop", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeights.Top, c))
			p.Add("borderRight", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeights.Right, c))
			p.Add("borderBottom", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeights.Bottom, c))
			p.Add("borderLeft", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeights.Left, c))
		case s.StrokeWeight.Set && !s.StrokeWeight.Mixed:
			p.Add("border", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeight.Value, c))
		}
	}

	for _, e := range s.Effects {
		switch e.Type {
		case "BACKGROUND_BLUR":
			if e.Radius > 0 {
				p.Add("backdropFilter", q(fmt.Sprintf("blur(%gpx)", e.Radius)))
			}
		case "LAYER_BLUR":
			if e.Radius > 0 {
				p.Add("filter", q(fmt.Sprintf("blur(%gpx)", e.Radius)))
			}
		}
	}

	if s.Opacity != nil && *s.Opacity < 1 {
		p.Add("opacity", fmt.Sprintf("%g", *s.Opacity))
	}

	if s.Rotation != nil && *s.Rotation != 0 {
		// Figma's rotation is positive = counter-clockwise; CSS rotate() is
		// positive = clockwise (even Figma's own Dev Mode panel emits a
		// negated value for this exact reason) — negate, or every rotated
		// element comes out spun the wrong way.
		p.Add("transform", fmt.Sprintf("'rotate(%gdeg)'", -*s.Rotation))
	}

	if s.ClipsContent != nil && *s.ClipsContent {
		p.Add("overflow", q("hidden"))
	}

	return p
}

// addConstrainedOffsets adds left/top (or right/bottom, when the node's
// Constraints pin it to the parent's far edge) for an absolutely-positioned
// node. Falls back to left/top when there's no constraint info or no usable
// parentBounds — same behavior as before Constraints were read.
func addConstrainedOffsets(p *ir.Style, s *figma.Style, bounds, parentBounds figma.Bounds) {
	var horiz, vert string
	if s != nil && s.Constraints != nil {
		horiz, vert = s.Constraints.Horizontal, s.Constraints.Vertical
	}
	if horiz == "MAX" && parentBounds.Width > 0 {
		p.Add("right", px(parentBounds.Width-(bounds.X+bounds.Width)))
	} else {
		p.Add("left", px(bounds.X))
	}
	if vert == "MAX" && parentBounds.Height > 0 {
		p.Add("bottom", px(parentBounds.Height-(bounds.Y+bounds.Height)))
	} else {
		p.Add("top", px(bounds.Y))
	}
}

// addGridPlacement adds gridRow/gridColumn for a node with an explicit
// position within its parent's GRID auto-layout. CSS grid lines are
// 1-indexed; Figma's row/column indices are 0-indexed.
func addGridPlacement(p *ir.Style, gp *figma.GridPosition) {
	if gp == nil {
		return
	}
	p.Add("gridRow", q(fmt.Sprintf("%d / span %d", gp.RowIndex+1, gp.RowSpan)))
	p.Add("gridColumn", q(fmt.Sprintf("%d / span %d", gp.ColumnIndex+1, gp.ColumnSpan)))
}

// cssTrack renders one GridTrack as a CSS grid-template track value.
func cssTrack(t figma.GridTrack) string {
	switch t.Type {
	case "FIXED":
		if t.Value != nil {
			return fmt.Sprintf("%gpx", *t.Value)
		}
	case "FLEX":
		v := 1.0
		if t.Value != nil {
			v = *t.Value
		}
		return fmt.Sprintf("%gfr", v)
	case "HUG":
		return "max-content"
	}
	return "auto"
}

// cssTrackList renders a GRID frame's row/column tracks as a CSS
// grid-template-rows/columns value, e.g. "100px 1fr max-content".
func cssTrackList(tracks []figma.GridTrack) string {
	parts := make([]string, len(tracks))
	for i, t := range tracks {
		parts[i] = cssTrack(t)
	}
	return strings.Join(parts, " ")
}

func buildTextStyle(s *figma.Style, bounds figma.Bounds, absolute bool, parentBounds figma.Bounds) *ir.Style {
	p := &ir.Style{}
	if absolute {
		p.Add("position", q("absolute"))
		addConstrainedOffsets(p, s, bounds, parentBounds)
		if bounds.Width > 0 {
			p.Add("width", px(bounds.Width))
		}
	}
	if s == nil {
		return p
	}
	if s.FontSize.Set && !s.FontSize.Mixed {
		p.Add("fontSize", px(s.FontSize.Value))
	}
	if s.FontWeight.Set && !s.FontWeight.Mixed {
		p.Add("fontWeight", fmt.Sprintf("%g", s.FontWeight.Value))
	}
	if s.FontFamily != "" {
		p.Add("fontFamily", q(s.FontFamily))
	}
	if c := figma.FirstSolidCSS(s.Fills.Paints); c != "" {
		p.Add("color", q(c))
	}
	if lh := s.LineHeight; lh != nil && lh.Unit == "PIXELS" {
		p.Add("lineHeight", px(lh.Value))
	}
	if ls := s.LetterSpacing; ls != nil && ls.Unit == "PIXELS" && ls.Value != 0 {
		p.Add("letterSpacing", px(ls.Value))
	}
	switch s.TextAlignHorizontal {
	case "CENTER":
		p.Add("textAlign", q("center"))
	case "RIGHT":
		p.Add("textAlign", q("right"))
	case "JUSTIFIED":
		p.Add("textAlign", q("justify"))
	}
	switch s.TextCase {
	case "UPPER":
		p.Add("textTransform", q("uppercase"))
	case "LOWER":
		p.Add("textTransform", q("lowercase"))
	case "TITLE":
		p.Add("textTransform", q("capitalize"))
	}
	return p
}

func buildShapeStyle(s *figma.Style, bounds figma.Bounds, gridPos *figma.GridPosition) *ir.Style {
	p := &ir.Style{}
	// A grid child must stay in-flow (not absolute) for gridRow/gridColumn
	// placement — added separately by addGridPlacement — to take effect.
	if gridPos == nil {
		p.Add("position", q("absolute"))
	}
	p.Add("width", px(bounds.Width))
	p.Add("height", px(bounds.Height))
	if s != nil {
		if c := figma.FirstSolidCSS(s.Fills.Paints); c != "" {
			p.Add("background", q(c))
		}
		if s.CornerRadius.Set && !s.CornerRadius.Mixed && s.CornerRadius.Value > 0 {
			if s.CornerRadius.Value >= 9999 {
				p.Add("borderRadius", q("9999px"))
			} else {
				p.Add("borderRadius", px(s.CornerRadius.Value))
			}
		}
	}
	return p
}

func px(v float64) string { return fmt.Sprintf("'%.4gpx'", v) }
func q(s string) string   { return "'" + s + "'" }

func pascalCase(name string) string {
	var sb strings.Builder
	capitalizeNext := true
	for _, r := range name {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if capitalizeNext && r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 32)
			} else {
				sb.WriteRune(r)
			}
			capitalizeNext = false
		default:
			capitalizeNext = true
		}
	}
	result := sb.String()
	if result == "" {
		return "GeneratedComponent"
	}
	// Must start with a letter.
	for i, r := range result {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return result[i:]
		}
	}
	return "GeneratedComponent"
}

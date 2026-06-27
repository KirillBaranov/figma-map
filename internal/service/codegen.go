package service

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/figma"
)

// CodegenResult is the TSX file generated for a Figma frame.
type CodegenResult struct {
	NodeID    string `json:"nodeId"`
	Name      string `json:"name"`
	Component string `json:"component"` // exported function name
	TSX       string `json:"tsx"`
}

// Codegen walks the full Figma node tree and emits a ready-to-edit TSX file.
// Every FRAME, GROUP, TEXT, RECTANGLE and INSTANCE node is translated to JSX
// with inline styles derived from Figma auto-layout, fills, and typography.
// INSTANCE nodes matched against the binding are emitted as UIKit components.
func (s *Service) Codegen(ctx context.Context, fileKey, nodeID, bindingPath string) (CodegenResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return CodegenResult{}, err
	}

	node, err := s.src.Node(ctx, key, nodeID)
	if err != nil {
		return CodegenResult{}, err
	}

	b, _ := binding.Load(bindingPath) // non-fatal: proceed without UIKit mapping

	// node.Bounds.X/Y are absolute canvas coordinates. The generated component
	// is meant to be embeddable on its own, not glued to its original canvas
	// position, so the root renders at its own origin; descendants keep their
	// parent-relative bounds untouched.
	root := *node
	root.Bounds.X, root.Bounds.Y = 0, 0

	gen := &codeGen{b: b, ctx: ctx, src: s.src, fileKey: key}
	body := gen.node(&root, 2, false, root.Bounds)

	compName := pascalCase(node.Name)
	var sb strings.Builder

	// Imports — sorted for stable output.
	syms := make([]string, 0, len(gen.used))
	for sym := range gen.used {
		syms = append(syms, sym)
	}
	sort.Strings(syms)
	for _, sym := range syms {
		fmt.Fprintf(&sb, "import { %s } from %q\n", sym, gen.used[sym])
	}
	if len(syms) > 0 {
		sb.WriteByte('\n')
	}

	fmt.Fprintf(&sb, "export function %s() {\n  return (\n", compName)
	sb.WriteString(body)
	sb.WriteString("\n  )\n}\n")

	return CodegenResult{NodeID: node.ID, Name: node.Name, Component: compName, TSX: sb.String()}, nil
}

// codeGen holds per-run state while walking the Figma tree. When html is
// true, the same tree-walk and CSS computation emit a standalone HTML
// document (real style="" attributes, no JSX, instances never substituted
// for a UIKit component) instead of the editable TSX — see Render.
type codeGen struct {
	b    binding.Binding
	used map[string]string // UIKit symbol → import path
	html bool

	// ctx/src/fileKey are only set for the real Codegen path (not Render's
	// HTML preview, which stays network-free and fast by design) — used to
	// export vector nodes to SVG files instead of dropping them as comments.
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

// node renders one Figma node and its entire subtree to indented TSX.
// parentHasAutoLayout=true means the parent is a flex container — children are
// in flow and must not be absolutely positioned. false means the parent uses
// position:relative, so children need position:absolute + left/top (or
// right/bottom, see buildFrameStyle's Constraints handling) to place
// themselves correctly. parentBounds is the immediate parent's Figma bounds,
// needed to compute a right/bottom offset from a MAX constraint.
func (g *codeGen) node(n *figma.Node, depth int, parentHasAutoLayout bool, parentBounds figma.Bounds) string {
	if n.Styles != nil && n.Styles.Visible != nil && !*n.Styles.Visible {
		return ind(depth) + g.comment(fmt.Sprintf("hidden: %s", n.Name))
	}

	absolute := !parentHasAutoLayout

	switch n.Type {
	case "TEXT":
		return g.text(n, depth, absolute, parentBounds)
	case "INSTANCE":
		// HTML preview mode never substitutes a UIKit component — there's no
		// app mounted to render it — so instances always fall through as
		// plain frames there.
		if !g.html {
			if comp, ok := g.findComp(n.Name); ok {
				return g.component(n, comp, depth)
			}
		}
		return g.frame(n, depth, absolute, parentBounds)
	case "VECTOR", "BOOLEAN_OPERATION", "STAR", "POLYGON":
		return g.vector(n, depth)
	default:
		if len(n.Children) > 0 || n.Type == "GROUP" || n.Type == "FRAME" ||
			n.Type == "COMPONENT" || n.Type == "COMPONENT_SET" {
			return g.frame(n, depth, absolute, parentBounds)
		}
		return g.shape(n, depth)
	}
}

func (g *codeGen) text(n *figma.Node, depth int, absolute bool, parentBounds figma.Bounds) string {
	p := buildTextStyle(n.Styles, n.Bounds, absolute, parentBounds)
	addGridPlacement(p, n.GridPosition)
	attr := g.styleAttr(p)
	text := strings.ReplaceAll(n.Characters, "\n", " ")
	if g.html {
		text = html.EscapeString(text)
	} else {
		text = strings.ReplaceAll(text, "{", "&#123;")
		text = strings.ReplaceAll(text, "}", "&#125;")
	}

	tag := "span"
	if n.Styles != nil && n.Styles.FontSize.Set && n.Styles.FontSize.Value >= 24 {
		tag = "p"
	}

	return ind(depth) + fmt.Sprintf("<%s%s>%s</%s>", tag, attr, text, tag)
}

func (g *codeGen) frame(n *figma.Node, depth int, absolute bool, parentBounds figma.Bounds) string {
	p := buildFrameStyle(n.Styles, n.Bounds, absolute, parentBounds)
	addGridPlacement(p, n.GridPosition)
	attr := g.styleAttr(p)

	// Children are in flow when THIS frame has autolayout.
	childrenInFlow := n.Styles != nil && n.Styles.AutoLayout != nil

	if len(n.Children) == 0 {
		return ind(depth) + fmt.Sprintf("<div%s />", attr)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s<div%s>\n", ind(depth), attr)
	for i := range n.Children {
		child := &n.Children[i]
		if child.Type != "TEXT" {
			fmt.Fprintf(&sb, "%s%s\n", ind(depth+1), g.comment(child.Name))
		}
		// A child can opt out of the parent's auto-layout flow via Figma's
		// "ABSOLUTE" escape hatch even when the parent itself is auto-layout.
		notAbsoluteChild := child.Styles == nil || child.Styles.LayoutPositioning != "ABSOLUTE"
		childInFlow := childrenInFlow && notAbsoluteChild
		sb.WriteString(g.node(child, depth+1, childInFlow, n.Bounds))
		sb.WriteByte('\n')
	}
	fmt.Fprintf(&sb, "%s</div>", ind(depth))
	return sb.String()
}

func (g *codeGen) shape(n *figma.Node, depth int) string {
	p := buildShapeStyle(n.Styles, n.Bounds, n.GridPosition)
	addGridPlacement(p, n.GridPosition)
	attr := g.styleAttr(p)
	return ind(depth) + fmt.Sprintf("<div%s />", attr)
}

// vector renders a VECTOR/BOOLEAN_OPERATION/STAR/POLYGON leaf node. In the
// real Codegen path (not Render's HTML preview, which stays network-free by
// design) it exports the node to an SVG file and emits an <img>. HTML-preview
// mode and any export failure fall back to a comment — a failed icon export
// shouldn't fail the whole codegen.
func (g *codeGen) vector(n *figma.Node, depth int) string {
	if !g.html && g.src != nil {
		if path, ok := g.exportVectorSVG(n); ok {
			p := buildShapeStyle(n.Styles, n.Bounds, n.GridPosition)
			addGridPlacement(p, n.GridPosition)
			attr := g.styleAttr(p)
			return ind(depth) + fmt.Sprintf("<img src=%q%s />", path, attr)
		}
	}
	return ind(depth) + g.comment(fmt.Sprintf("SVG: %s", n.Name))
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

// comment renders an aside (a skipped node, a hidden node) as a JSX comment
// or an HTML comment, matching the current output mode.
func (g *codeGen) comment(text string) string {
	if g.html {
		return fmt.Sprintf("<!-- %s -->", text)
	}
	return fmt.Sprintf("{/* %s */}", text)
}

// styleAttr renders a style attribute in the current output mode: a JSX
// object literal (style={{ key: val }}) or a real HTML attribute
// (style="key: val"). Empty when there are no properties.
func (g *codeGen) styleAttr(p *styleProps) string {
	if len(p.keys) == 0 {
		return ""
	}
	if g.html {
		return fmt.Sprintf(` style="%s"`, p.renderCSSAttr())
	}
	return " style=" + p.renderJSX()
}

func (g *codeGen) component(n *figma.Node, comp binding.Component, depth int) string {
	g.addImport(comp.Symbol, comp.Import)

	// Default props from binding — agent will refine.
	var attrs []string
	for name, prop := range comp.Props {
		if def := prop.Default(); def != "" {
			attrs = append(attrs, fmt.Sprintf("%s=%q", name, def))
		}
	}
	sort.Strings(attrs)

	attrStr := ""
	if len(attrs) > 0 {
		attrStr = " " + strings.Join(attrs, " ")
	}

	sym := comp.Symbol
	if text := n.FirstText(); text != "" {
		return ind(depth) + fmt.Sprintf("<%s%s>%s</%s>", sym, attrStr, text, sym)
	}
	return ind(depth) + fmt.Sprintf("<%s%s />", sym, attrStr)
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

func buildFrameStyle(s *figma.Style, bounds figma.Bounds, absolute bool, parentBounds figma.Bounds) *styleProps {
	p := &styleProps{}

	// Position and placement.
	if absolute {
		p.add("position", q("absolute"))
		addConstrainedOffsets(p, s, bounds, parentBounds)
	}

	if s == nil {
		if !absolute {
			p.add("position", q("relative"))
		}
		p.add("width", px(bounds.Width))
		p.add("height", px(bounds.Height))
		return p
	}

	if al := s.AutoLayout; al != nil && al.Direction == "GRID" {
		// Figma's native CSS-grid-like auto-layout — the designer's own
		// explicit row/column setup, surfaced as-is, never inferred from
		// freeform positioning.
		p.add("display", q("grid"))
		if len(al.GridRowSizes) > 0 {
			p.add("gridTemplateRows", q(cssTrackList(al.GridRowSizes)))
		}
		if len(al.GridColumnSizes) > 0 {
			p.add("gridTemplateColumns", q(cssTrackList(al.GridColumnSizes)))
		}
		if al.GridRowGap == al.GridColumnGap {
			if al.GridRowGap > 0 {
				p.add("gap", px(al.GridRowGap))
			}
		} else {
			if al.GridRowGap > 0 {
				p.add("rowGap", px(al.GridRowGap))
			}
			if al.GridColumnGap > 0 {
				p.add("columnGap", px(al.GridColumnGap))
			}
		}
	} else if al := s.AutoLayout; al != nil {
		p.add("display", q("flex"))
		switch al.Direction {
		case "HORIZONTAL":
			p.add("flexDirection", q("row"))
		case "VERTICAL":
			p.add("flexDirection", q("column"))
		}
		if al.Gap > 0 {
			p.add("gap", px(al.Gap))
		}
		switch al.PrimaryAxisAlign {
		case "CENTER":
			p.add("justifyContent", q("center"))
		case "MAX":
			p.add("justifyContent", q("flex-end"))
		case "SPACE_BETWEEN":
			p.add("justifyContent", q("space-between"))
		}
		switch al.CounterAxisAlign {
		case "CENTER":
			p.add("alignItems", q("center"))
		case "MAX":
			p.add("alignItems", q("flex-end"))
		}
		if al.Wrap == "WRAP" {
			p.add("flexWrap", q("wrap"))
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
			p.add("width", px(bounds.Width))
		}
		if heightFixed && bounds.Height > 0 {
			p.add("height", px(bounds.Height))
		}
	} else {
		if !absolute {
			p.add("position", q("relative"))
		}
		if bounds.Width > 0 {
			p.add("width", px(bounds.Width))
		}
		if bounds.Height > 0 {
			p.add("height", px(bounds.Height))
		}
	}

	if pad := s.Padding; pad != nil {
		switch {
		case pad.Top == pad.Right && pad.Right == pad.Bottom && pad.Bottom == pad.Left:
			p.add("padding", px(pad.Top))
		case pad.Top == pad.Bottom && pad.Left == pad.Right:
			p.add("padding", fmt.Sprintf("'%gpx %gpx'", pad.Top, pad.Left))
		default:
			p.add("paddingTop", px(pad.Top))
			p.add("paddingRight", px(pad.Right))
			p.add("paddingBottom", px(pad.Bottom))
			p.add("paddingLeft", px(pad.Left))
		}
	}

	if c := figma.FirstSolidCSS(s.Fills.Paints); c != "" {
		p.add("background", q(c))
	}

	if s.CornerRadius.Set && !s.CornerRadius.Mixed && s.CornerRadius.Value > 0 {
		if s.CornerRadius.Value >= 9999 {
			p.add("borderRadius", q("9999px"))
		} else {
			p.add("borderRadius", px(s.CornerRadius.Value))
		}
	}

	if c := figma.FirstSolidCSS(s.Strokes.Paints); c != "" && len(s.Strokes.Paints) > 0 {
		switch {
		case s.StrokeWeights != nil:
			// Per-side weights differ — a single `border` shorthand can't
			// express that.
			p.add("borderTop", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeights.Top, c))
			p.add("borderRight", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeights.Right, c))
			p.add("borderBottom", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeights.Bottom, c))
			p.add("borderLeft", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeights.Left, c))
		case s.StrokeWeight.Set && !s.StrokeWeight.Mixed:
			p.add("border", fmt.Sprintf("'%gpx solid %s'", s.StrokeWeight.Value, c))
		}
	}

	for _, e := range s.Effects {
		switch e.Type {
		case "BACKGROUND_BLUR":
			if e.Radius > 0 {
				p.add("backdropFilter", q(fmt.Sprintf("blur(%gpx)", e.Radius)))
			}
		case "LAYER_BLUR":
			if e.Radius > 0 {
				p.add("filter", q(fmt.Sprintf("blur(%gpx)", e.Radius)))
			}
		}
	}

	if s.Opacity != nil && *s.Opacity < 1 {
		p.add("opacity", fmt.Sprintf("%g", *s.Opacity))
	}

	if s.Rotation != nil && *s.Rotation != 0 {
		p.add("transform", fmt.Sprintf("'rotate(%gdeg)'", *s.Rotation))
	}

	if s.ClipsContent != nil && *s.ClipsContent {
		p.add("overflow", q("hidden"))
	}

	return p
}

// addConstrainedOffsets adds left/top (or right/bottom, when the node's
// Constraints pin it to the parent's far edge) for an absolutely-positioned
// node. Falls back to left/top when there's no constraint info or no usable
// parentBounds — same behavior as before Constraints were read.
func addConstrainedOffsets(p *styleProps, s *figma.Style, bounds, parentBounds figma.Bounds) {
	var horiz, vert string
	if s != nil && s.Constraints != nil {
		horiz, vert = s.Constraints.Horizontal, s.Constraints.Vertical
	}
	if horiz == "MAX" && parentBounds.Width > 0 {
		p.add("right", px(parentBounds.Width-(bounds.X+bounds.Width)))
	} else if bounds.X != 0 {
		p.add("left", px(bounds.X))
	}
	if vert == "MAX" && parentBounds.Height > 0 {
		p.add("bottom", px(parentBounds.Height-(bounds.Y+bounds.Height)))
	} else if bounds.Y != 0 {
		p.add("top", px(bounds.Y))
	}
}

// addGridPlacement adds gridRow/gridColumn for a node with an explicit
// position within its parent's GRID auto-layout. CSS grid lines are
// 1-indexed; Figma's row/column indices are 0-indexed.
func addGridPlacement(p *styleProps, gp *figma.GridPosition) {
	if gp == nil {
		return
	}
	p.add("gridRow", q(fmt.Sprintf("%d / span %d", gp.RowIndex+1, gp.RowSpan)))
	p.add("gridColumn", q(fmt.Sprintf("%d / span %d", gp.ColumnIndex+1, gp.ColumnSpan)))
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

func buildTextStyle(s *figma.Style, bounds figma.Bounds, absolute bool, parentBounds figma.Bounds) *styleProps {
	p := &styleProps{}
	if absolute {
		p.add("position", q("absolute"))
		addConstrainedOffsets(p, s, bounds, parentBounds)
	}
	if s == nil {
		return p
	}
	if s.FontSize.Set && !s.FontSize.Mixed {
		p.add("fontSize", px(s.FontSize.Value))
	}
	if s.FontWeight.Set && !s.FontWeight.Mixed {
		p.add("fontWeight", fmt.Sprintf("%g", s.FontWeight.Value))
	}
	if s.FontFamily != "" {
		p.add("fontFamily", q(s.FontFamily))
	}
	if c := figma.FirstSolidCSS(s.Fills.Paints); c != "" {
		p.add("color", q(c))
	}
	if lh := s.LineHeight; lh != nil && lh.Unit == "PIXELS" {
		p.add("lineHeight", px(lh.Value))
	}
	if ls := s.LetterSpacing; ls != nil && ls.Unit == "PIXELS" && ls.Value != 0 {
		p.add("letterSpacing", px(ls.Value))
	}
	switch s.TextAlignHorizontal {
	case "CENTER":
		p.add("textAlign", q("center"))
	case "RIGHT":
		p.add("textAlign", q("right"))
	case "JUSTIFIED":
		p.add("textAlign", q("justify"))
	}
	switch s.TextCase {
	case "UPPER":
		p.add("textTransform", q("uppercase"))
	case "LOWER":
		p.add("textTransform", q("lowercase"))
	case "TITLE":
		p.add("textTransform", q("capitalize"))
	}
	return p
}

func buildShapeStyle(s *figma.Style, bounds figma.Bounds, gridPos *figma.GridPosition) *styleProps {
	p := &styleProps{}
	// A grid child must stay in-flow (not absolute) for gridRow/gridColumn
	// placement — added separately by addGridPlacement — to take effect.
	if gridPos == nil {
		p.add("position", q("absolute"))
	}
	p.add("width", px(bounds.Width))
	p.add("height", px(bounds.Height))
	if s != nil {
		if c := figma.FirstSolidCSS(s.Fills.Paints); c != "" {
			p.add("background", q(c))
		}
		if s.CornerRadius.Set && !s.CornerRadius.Mixed && s.CornerRadius.Value > 0 {
			if s.CornerRadius.Value >= 9999 {
				p.add("borderRadius", q("9999px"))
			} else {
				p.add("borderRadius", px(s.CornerRadius.Value))
			}
		}
	}
	return p
}

// styleProps is an insertion-ordered list of CSS property→value pairs.
// renderJSX renders a JSX style literal ({{ key: value, ... }}); renderCSSAttr
// renders the same properties as a real CSS declaration list for an HTML
// style="" attribute.
type styleProps struct {
	keys []string
	vals map[string]string
}

func (p *styleProps) add(k, v string) {
	if p.vals == nil {
		p.vals = map[string]string{}
	}
	if _, exists := p.vals[k]; !exists {
		p.keys = append(p.keys, k)
	}
	p.vals[k] = v
}

func (p *styleProps) renderJSX() string {
	if len(p.keys) == 0 {
		return "{{}}"
	}
	parts := make([]string, 0, len(p.keys))
	for _, k := range p.keys {
		parts = append(parts, k+": "+p.vals[k])
	}
	return "{{ " + strings.Join(parts, ", ") + " }}"
}

// renderCSSAttr unwraps each JS-literal value (strips the surrounding quotes
// our q()/px() helpers add — the CSS itself doesn't need them) and joins them
// as "key: value; key2: value2".
func (p *styleProps) renderCSSAttr() string {
	parts := make([]string, 0, len(p.keys))
	for _, k := range p.keys {
		v := strings.Trim(p.vals[k], `'"`)
		parts = append(parts, cssPropName(k)+": "+v)
	}
	return strings.Join(parts, "; ")
}

// cssPropName converts a camelCase JS style key to kebab-case CSS
// (backdropFilter → backdrop-filter).
func cssPropName(camel string) string {
	var sb strings.Builder
	for _, r := range camel {
		if r >= 'A' && r <= 'Z' {
			sb.WriteByte('-')
			sb.WriteRune(r - 'A' + 'a')
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func ind(depth int) string { return strings.Repeat("  ", depth) }
func px(v float64) string  { return fmt.Sprintf("'%.4gpx'", v) }
func q(s string) string    { return "'" + s + "'" }

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

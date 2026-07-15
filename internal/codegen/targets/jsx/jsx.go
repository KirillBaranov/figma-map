// Package jsx renders the codegen ir.Node tree as TSX: JSX markup with
// camelCase inline style object literals. This reproduces figma-map's
// original (and default) codegen output exactly — see
// internal/service/codegen.go for the tree-walk that builds the tree this
// package serializes.
package jsx

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/codegen"
	"github.com/kirillbaranov/figma-map/internal/codegen/ir"
)

func init() {
	codegen.Register("jsx", New())
}

// renderer is written for ir.Version 1.
var _ = ir.Version

type renderer struct{}

// New returns the jsx Renderer.
func New() codegen.Renderer { return &renderer{} }

// RenderFile wraps root as a ready-to-edit component file: sorted imports,
// then `export function <componentName>() { return (...) }`.
func (r *renderer) RenderFile(root *ir.Node, imports map[string]string, componentName string) string {
	var sb strings.Builder

	syms := make([]string, 0, len(imports))
	for sym := range imports {
		syms = append(syms, sym)
	}
	sort.Strings(syms)
	for _, sym := range syms {
		fmt.Fprintf(&sb, "import { %s } from %q\n", sym, imports[sym])
	}
	if len(syms) > 0 {
		sb.WriteByte('\n')
	}

	fmt.Fprintf(&sb, "export function %s() {\n  return (\n", componentName)
	sb.WriteString(r.RenderNode(root, 2))
	sb.WriteString("\n  )\n}\n")
	return sb.String()
}

// RenderNode serializes n and its subtree as indented JSX.
func (r *renderer) RenderNode(n *ir.Node, depth int) string {
	switch n.Kind {
	case ir.Comment:
		return ind(depth) + fmt.Sprintf("{/* %s */}", n.Text)
	case ir.Component:
		return r.renderComponent(n, depth)
	default:
		return r.renderElement(n, depth)
	}
}

func (r *renderer) renderElement(n *ir.Node, depth int) string {
	attr := styleAttr(n.Style)

	if isTextTag(n) {
		text := escapeText(n.Children[0].Text)
		return ind(depth) + fmt.Sprintf("<%s%s>%s</%s>", n.Tag, attr, text, n.Tag)
	}

	if n.Src != "" {
		return ind(depth) + fmt.Sprintf("<%s src=%q%s />", n.Tag, n.Src, attr)
	}

	if len(n.Children) == 0 {
		return ind(depth) + fmt.Sprintf("<%s%s />", n.Tag, attr)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s<%s%s>\n", ind(depth), n.Tag, attr)
	for _, child := range n.Children {
		sb.WriteString(r.RenderNode(child, depth+1))
		sb.WriteByte('\n')
	}
	fmt.Fprintf(&sb, "%s</%s>", ind(depth), n.Tag)
	return sb.String()
}

func (r *renderer) renderComponent(n *ir.Node, depth int) string {
	attrs := renderAttrs(n.Props)
	attrStr := ""
	if attrs != "" {
		attrStr = " " + attrs
	}
	if n.Text != "" {
		return ind(depth) + fmt.Sprintf("<%s%s>%s</%s>", n.Tag, attrStr, n.Text, n.Tag)
	}
	return ind(depth) + fmt.Sprintf("<%s%s />", n.Tag, attrStr)
}

func isTextTag(n *ir.Node) bool {
	return len(n.Children) == 1 && n.Children[0].Kind == ir.TextLeaf
}

// escapeText escapes JSX's two reserved characters so literal Figma text
// content never gets parsed as an expression container.
func escapeText(s string) string {
	s = strings.ReplaceAll(s, "{", "&#123;")
	s = strings.ReplaceAll(s, "}", "&#125;")
	return s
}

// styleAttr renders a JSX style object literal (style={{ key: val, ... }}).
// Empty when there are no properties.
func styleAttr(s *ir.Style) string {
	if s.Empty() {
		return ""
	}
	parts := make([]string, 0, len(s.Keys))
	for _, k := range s.Keys {
		parts = append(parts, k+": "+s.Vals[k])
	}
	return " style={{ " + strings.Join(parts, ", ") + " }}"
}

// renderAttrs formats props as JSX attributes in stable (sorted) order.
func renderAttrs(props map[string]string) string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, props[k]))
	}
	return strings.Join(parts, " ")
}

func ind(depth int) string { return strings.Repeat("  ", depth) }

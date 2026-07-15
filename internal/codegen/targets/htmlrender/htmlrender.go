// Package htmlrender renders the codegen ir.Node tree as standalone HTML:
// real style="" attributes (kebab-case CSS), HTML-entity text escaping, no
// component-import wrapper. This backs the network-free preview used by
// capture/pixeldiff to screenshot figma-map's own raw output without a
// running app — see internal/service/preview.go. Named htmlrender (not
// html) to avoid colliding with the stdlib html package it imports below.
package htmlrender

import (
	"fmt"
	"html"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/codegen"
	"github.com/kirillbaranov/figma-map/internal/codegen/ir"
)

func init() {
	codegen.Register("html", New())
}

// renderer is written for ir.Version 1.
var _ = ir.Version

type renderer struct{}

// New returns the "html" target Renderer.
func New() codegen.Renderer { return &renderer{} }

// RenderFile has no meaningful "file" shape for a raw HTML fragment target —
// callers needing a full document (see previewHTML) build their own
// surrounding <html>/<body> and embed RenderNode's output directly. This
// exists only to satisfy the Renderer interface.
func (r *renderer) RenderFile(root *ir.Node, _ map[string]string, _ string) string {
	return r.RenderNode(root, 0)
}

// RenderNode serializes n and its subtree as indented HTML.
func (r *renderer) RenderNode(n *ir.Node, depth int) string {
	switch n.Kind {
	case ir.Comment:
		return ind(depth) + fmt.Sprintf("<!-- %s -->", n.Text)
	case ir.Component:
		return r.renderComponent(n, depth)
	default:
		return r.renderElement(n, depth)
	}
}

func (r *renderer) renderElement(n *ir.Node, depth int) string {
	attr := styleAttr(n.Style)

	if isTextTag(n) {
		text := html.EscapeString(n.Children[0].Text)
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

// renderComponent handles a Component node the same as a plain element would
// (real HTML has no notion of a UIKit component) — in practice this never
// fires for the html target, since the tree-walk only produces Component
// nodes when a binding actually matched, and the preview path never loads
// one.
func (r *renderer) renderComponent(n *ir.Node, depth int) string {
	attr := styleAttr(n.Style)
	tag := "div"
	if n.Text != "" {
		return ind(depth) + fmt.Sprintf("<%s%s>%s</%s>", tag, attr, html.EscapeString(n.Text), tag)
	}
	return ind(depth) + fmt.Sprintf("<%s%s />", tag, attr)
}

func isTextTag(n *ir.Node) bool {
	return len(n.Children) == 1 && n.Children[0].Kind == ir.TextLeaf
}

// styleAttr unwraps each JS-literal value (strips the surrounding quotes the
// tree-walk's px()/q() helpers add — real CSS doesn't need them) and renders
// a real HTML style="" attribute in kebab-case.
func styleAttr(s *ir.Style) string {
	if s.Empty() {
		return ""
	}
	parts := make([]string, 0, len(s.Keys))
	for _, k := range s.Keys {
		v := strings.Trim(s.Vals[k], `'"`)
		parts = append(parts, cssPropName(k)+": "+v)
	}
	return fmt.Sprintf(` style="%s"`, strings.Join(parts, "; "))
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

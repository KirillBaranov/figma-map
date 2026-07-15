// Package ir defines the target-neutral tree that Service.Codegen builds by
// walking a Figma node. It has zero dependencies on Figma types or on any
// specific output target (JSX, HTML, or future Vue/Svelte/Angular) — that
// separation is the whole point: the tree-walk in internal/service/codegen.go
// builds this once, and each internal/codegen/targets/* package serializes it
// independently.
package ir

// Version is the contract version renderer packages are written against.
// Fields may only be added within a version — never renamed, removed, or
// repurposed. A breaking change to this shape bumps Version, which every
// Renderer implementation should reference (e.g. a comment or a compile-time
// assertion) so an incompatible change is a visible break, not a silent one.
const Version = 1

// Kind discriminates what a Node represents.
type Kind int

const (
	// Element is a plain markup node: a tag, optional style, and children
	// (or self-closing when there are none) — e.g. a div/span/p/img.
	Element Kind = iota
	// TextLeaf is literal text content. Text holds the raw, unescaped
	// characters; each renderer applies its own escaping.
	TextLeaf
	// Comment is an aside for a skipped/hidden Figma node, or a fallback for
	// a vector that couldn't be exported. Text holds the raw comment body.
	Comment
	// Component is a matched UIKit binding instance: Tag is the component
	// symbol, Import names where to import it from, and Props are the
	// resolved attribute values.
	Component
)

// Import names a symbol to import and the module path to import it from.
type Import struct {
	Symbol string
	Path   string
}

// Node is one element of the target-neutral tree. Depth/indentation is a
// rendering concern, not part of this shape — the tree-walk that builds Node
// values never needs to know how deep it is.
type Node struct {
	Kind Kind

	// Tag is the element tag ("div", "span", "p", "img") for Element, or the
	// bound component's symbol for Component. Unused for TextLeaf/Comment.
	Tag string

	// Style is an ordered set of CSS-ish property/value pairs, target-neutral
	// (e.g. key "fontSize", value "'14px'"). nil or empty means no style.
	Style *Style

	// Text is the literal content for TextLeaf, or the comment body for
	// Comment. Unused otherwise.
	Text string

	// Src is an Element's image source (e.g. an exported SVG file path).
	Src string

	// Import and Props are set only for Component nodes.
	Import *Import
	Props  map[string]string

	SelfClose bool
	Children  []*Node
}

// Style is an insertion-ordered list of CSS property/value pairs. Ordering is
// preserved (not sorted) so renderer output stays stable across runs.
type Style struct {
	Keys []string
	Vals map[string]string
}

// Add appends k/v, or overwrites v in place if k was already added.
func (s *Style) Add(k, v string) {
	if s.Vals == nil {
		s.Vals = map[string]string{}
	}
	if _, exists := s.Vals[k]; !exists {
		s.Keys = append(s.Keys, k)
	}
	s.Vals[k] = v
}

// Empty reports whether the style has no properties.
func (s *Style) Empty() bool {
	return s == nil || len(s.Keys) == 0
}

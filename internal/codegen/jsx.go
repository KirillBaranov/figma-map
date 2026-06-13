// Package codegen renders a JSX snippet from a matched component binding and a
// set of inferred prop values. It is pure and deterministic: same inputs always
// produce the same output.
package codegen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/binding"
)

// Element is everything needed to render one component instance.
type Element struct {
	// Component is the resolved binding for the matched component.
	Component binding.Component
	// Props are the chosen code prop -> value pairs (only non-default values
	// need be present; defaults may be omitted by the caller).
	Props map[string]string
	// Children is the text content to place between the tags, if any.
	Children string
}

// JSX renders the element as an import line plus a JSX tag.
func JSX(el Element) string {
	var b strings.Builder

	fmt.Fprintf(&b, "import { %s } from %q\n\n", el.Component.Symbol, el.Component.Import)

	tag := el.Component.Symbol
	attrs := renderAttrs(el.Props)

	if el.Children == "" {
		if attrs == "" {
			fmt.Fprintf(&b, "<%s />", tag)
		} else {
			fmt.Fprintf(&b, "<%s %s />", tag, attrs)
		}
		return b.String()
	}

	if attrs == "" {
		fmt.Fprintf(&b, "<%s>%s</%s>", tag, el.Children, tag)
	} else {
		fmt.Fprintf(&b, "<%s %s>%s</%s>", tag, attrs, el.Children, tag)
	}
	return b.String()
}

// renderAttrs formats props as JSX attributes in stable (sorted) order. Empty
// values are skipped.
func renderAttrs(props map[string]string) string {
	keys := make([]string, 0, len(props))
	for k, v := range props {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, props[k]))
	}
	return strings.Join(parts, " ")
}

package codegen

import (
	"sort"

	"github.com/kirillbaranov/figma-map/internal/codegen/ir"
)

// Renderer serializes an ir.Node tree produced by Service.Codegen's tree-walk
// into a specific target's source text. Implementations live under
// internal/codegen/targets/<name> and self-register via Register in an
// init() — this package never imports a target package, so the core (this
// file + ir) compiles and is fully testable with zero targets present.
type Renderer interface {
	// RenderNode serializes n and its subtree, indented as if it started at
	// the given depth. Used both for a full document (depth 2, the JSX
	// target's `export function` body) and standalone snippets (depth 0,
	// e.g. Plan's unmapped-instance preview).
	RenderNode(n *ir.Node, depth int) string

	// RenderFile wraps root as a complete, ready-to-edit file: imports,
	// component declaration, and whatever else the target's file shape
	// needs. imports maps an imported symbol to its module path; componentName
	// is the exported symbol name for the generated component.
	RenderFile(root *ir.Node, imports map[string]string, componentName string) string
}

var registry = map[string]Renderer{}

// Register adds a Renderer under name, overwriting any previous registration
// for that name. Called from a target package's init().
func Register(name string, r Renderer) {
	registry[name] = r
}

// Get looks up a registered Renderer by name.
func Get(name string) (Renderer, bool) {
	r, ok := registry[name]
	return r, ok
}

// Registered returns every registered target name, sorted — used for
// --target help text and "unknown target" error messages, so it's always the
// single source of truth for what's currently supported.
func Registered() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

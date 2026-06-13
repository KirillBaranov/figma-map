package op

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/service"
)

// All returns every operation, in display order. Each appears once and yields
// both a CLI subcommand and an MCP tool.
func All() []Registrar {
	return []Registrar{
		doctorOp,
		scanOp,
		bindOp,
		listOp,
		mapOp,
	}
}

// ---- doctor ----

type doctorIn struct{}

var doctorOp = Op[doctorIn, service.Report]{
	Name:    "doctor",
	Summary: "Check that the bridge, Chrome, Storybook, and API key are available",
	Run: func(_ context.Context, s *service.Service, _ doctorIn) (service.Report, error) {
		return s.Doctor(), nil
	},
	Render: func(r service.Report) string {
		var b strings.Builder
		b.WriteString("figma-map doctor\n")
		for _, c := range r.Checks {
			if c.OK {
				fmt.Fprintf(&b, "  ✓ %s\n", c.Name)
			} else {
				fmt.Fprintf(&b, "  ✗ %s — %s\n", c.Name, c.Detail)
			}
		}
		if r.OK {
			b.WriteString("all checks passed")
		} else {
			b.WriteString("one or more checks failed")
		}
		return b.String()
	},
	Status: func(r service.Report) error {
		if !r.OK {
			return fmt.Errorf("one or more checks failed")
		}
		return nil
	},
}

// ---- scan ----

type scanIn struct {
	Storybook string `json:"storybook" jsonschema:"Storybook base URL (default from config)"`
	Project   string `json:"project" jsonschema:"project root with *.stories source files" default:"."`
	Out       string `json:"out" jsonschema:"output catalog directory" default:"catalog"`
}

var scanOp = Op[scanIn, service.ScanResult]{
	Name:    "scan",
	Summary: "Screenshot Storybook stories into a code-component catalog",
	Run: func(ctx context.Context, s *service.Service, in scanIn) (service.ScanResult, error) {
		return s.Scan(ctx, in.Storybook, in.Project, in.Out)
	},
	Render: func(r service.ScanResult) string {
		return fmt.Sprintf("Wrote %s/catalog.json (%d stories)", r.Out, r.Stories)
	},
}

// ---- bind ----

type bindIn struct {
	File    string `json:"file" jsonschema:"Figma file key (default: config or sole connected file)"`
	Catalog string `json:"catalog" jsonschema:"catalog directory from scan" default:"catalog"`
	Out     string `json:"out" jsonschema:"output binding file" default:"figma-map.binding.yaml"`
}

var bindOp = Op[bindIn, service.BindResult]{
	Name:    "bind",
	Summary: "Match Figma component sections to the catalog and write a binding",
	Long: "bind screenshots each top-level Figma component section, matches it " +
		"against the catalog with a vision LLM, infers each matched component's " +
		"prop schema, and writes a reviewable figma-map.binding.yaml.",
	Run: func(ctx context.Context, s *service.Service, in bindIn) (service.BindResult, error) {
		return s.Bind(ctx, in.File, in.Catalog, in.Out)
	},
	Render: func(r service.BindResult) string {
		return fmt.Sprintf("Wrote %s (%d components): %s. Review before use.",
			r.Out, len(r.Components), strings.Join(r.Components, ", "))
	},
}

// ---- list ----

type listIn struct {
	Binding string `json:"binding" jsonschema:"binding file from bind" default:"figma-map.binding.yaml"`
}

var listOp = Op[listIn, service.ListResult]{
	Name:    "list",
	Summary: "List the components in a binding",
	Run: func(_ context.Context, s *service.Service, in listIn) (service.ListResult, error) {
		return s.List(in.Binding)
	},
	Render: func(r service.ListResult) string {
		if len(r.Components) == 0 {
			return "no components"
		}
		var b strings.Builder
		for _, c := range r.Components {
			props := make([]string, 0, len(c.Props))
			for name := range c.Props {
				props = append(props, name)
			}
			sort.Strings(props)
			fmt.Fprintf(&b, "%-16s %s {%s}\n", c.Name, c.Import, strings.Join(props, ", "))
		}
		return strings.TrimRight(b.String(), "\n")
	},
}

// ---- map ----

type mapIn struct {
	NodeID  string `json:"nodeId" jsonschema:"Figma node id to map" cli:"arg"`
	File    string `json:"file" jsonschema:"Figma file key (default: config or sole connected file)"`
	Binding string `json:"binding" jsonschema:"binding file from bind" default:"figma-map.binding.yaml"`
	Catalog string `json:"catalog" jsonschema:"catalog directory from scan" default:"catalog"`
}

var mapOp = Op[mapIn, service.MapResult]{
	Name:    "map",
	Summary: "Generate code for a Figma node using a binding",
	Run: func(ctx context.Context, s *service.Service, in mapIn) (service.MapResult, error) {
		return s.Map(ctx, in.File, in.Binding, in.Catalog, in.NodeID)
	},
	Render: func(r service.MapResult) string {
		return fmt.Sprintf("// %s → %s (%.2f)\n%s", r.NodeID, r.Component, r.Score, r.JSX)
	},
}

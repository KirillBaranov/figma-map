package op

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/service"
)

// indentJSON pretty-prints a value for human CLI output.
func indentJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// All returns every operation, in display order. Each appears once and yields
// both a CLI subcommand and an MCP tool.
func All() []Registrar {
	return []Registrar{
		doctorOp,
		scanOp,
		bindOp,
		listOp,
		tokensOp,
		inspectOp,
		screenshotOp,
		exportAssetsOp,
		mapOp,
		planOp,
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

// ---- tokens ----

type tokensIn struct {
	NodeID string `json:"nodeId" jsonschema:"Figma node id" cli:"arg"`
	File   string `json:"file" jsonschema:"Figma file key (default: config or sole connected file)"`
}

var tokensOp = Op[tokensIn, service.TokensResult]{
	Name:    "tokens",
	Summary: "Extract exact design tokens (color, spacing, font, radius) for a node",
	Run: func(ctx context.Context, s *service.Service, in tokensIn) (service.TokensResult, error) {
		return s.GetTokens(ctx, in.File, in.NodeID)
	},
	Render: func(r service.TokensResult) string {
		if r.Tokens == nil {
			return fmt.Sprintf("%s (%s): no tokens", r.Name, r.Type)
		}
		return fmt.Sprintf("%s (%s):\n%s", r.Name, r.Type, indentJSON(r.Tokens))
	},
}

// ---- inspect ----

type inspectIn struct {
	NodeID string `json:"nodeId" jsonschema:"Figma node id" cli:"arg"`
	File   string `json:"file" jsonschema:"Figma file key (default: config or sole connected file)"`
	Tokens bool   `json:"tokens" jsonschema:"include design tokens per node"`
	Depth  int    `json:"depth" jsonschema:"max tree depth (0 = unlimited)" default:"0"`
}

var inspectOp = Op[inspectIn, service.InspectResult]{
	Name:    "inspect",
	Summary: "Inspect a Figma node subtree (structure, text, bounds, optional tokens)",
	Run: func(ctx context.Context, s *service.Service, in inspectIn) (service.InspectResult, error) {
		return s.Inspect(ctx, in.File, in.NodeID, in.Tokens, in.Depth)
	},
	Render: func(r service.InspectResult) string {
		var b strings.Builder
		for _, n := range r.Nodes {
			indent := strings.Repeat("  ", n.Depth)
			fmt.Fprintf(&b, "%s%s [%s]", indent, n.Name, n.Type)
			if n.Text != "" {
				fmt.Fprintf(&b, " %q", n.Text)
			}
			b.WriteString("\n")
		}
		return strings.TrimRight(b.String(), "\n")
	},
}

// ---- screenshot ----

type screenshotIn struct {
	NodeID string  `json:"nodeId" jsonschema:"Figma node id" cli:"arg"`
	File   string  `json:"file" jsonschema:"Figma file key (default: config or sole connected file)"`
	Out    string  `json:"out" jsonschema:"output PNG path (CLI); omit for raw image (MCP)"`
	Scale  float64 `json:"scale" jsonschema:"export scale factor" default:"2"`
}

var screenshotOp = Op[screenshotIn, service.ScreenshotResult]{
	Name:    "screenshot",
	Summary: "Render a Figma node to a PNG image",
	Run: func(ctx context.Context, s *service.Service, in screenshotIn) (service.ScreenshotResult, error) {
		return s.Screenshot(ctx, in.File, in.NodeID, in.Scale, in.Out)
	},
	Render: func(r service.ScreenshotResult) string {
		if r.Path != "" {
			return fmt.Sprintf("wrote %s (%d×%d)", r.Path, r.Width, r.Height)
		}
		return fmt.Sprintf("%d×%d PNG — pass --out to save, or use MCP for the image", r.Width, r.Height)
	},
	Image: func(r service.ScreenshotResult) ([]byte, string) {
		return r.PNG, "image/png"
	},
}

// ---- export-assets ----

type exportIn struct {
	NodeID string `json:"nodeId" jsonschema:"Figma node id to export" cli:"arg"`
	File   string `json:"file" jsonschema:"Figma file key (default: config or sole connected file)"`
	Format string `json:"format" jsonschema:"PNG, SVG, or JPG" default:"SVG"`
	Out    string `json:"out" jsonschema:"output directory" default:"assets"`
}

var exportAssetsOp = Op[exportIn, service.ExportResult]{
	Name:    "export-assets",
	Summary: "Export a Figma node to a file (SVG/PNG/JPG) for use as a production asset",
	Run: func(ctx context.Context, s *service.Service, in exportIn) (service.ExportResult, error) {
		return s.ExportAssets(ctx, in.File, in.NodeID, in.Format, in.Out)
	},
	Render: func(r service.ExportResult) string {
		return fmt.Sprintf("exported %s (%d bytes)", r.Path, r.Bytes)
	},
}

// ---- plan ----

type planIn struct {
	FrameID string `json:"frameId" jsonschema:"Figma frame node id to map" cli:"arg"`
	File    string `json:"file" jsonschema:"Figma file key (default: config or sole connected file)"`
	Depth   int    `json:"depth" jsonschema:"max nesting depth to search (0 = unlimited)" default:"0"`
	Binding string `json:"binding" jsonschema:"binding file from bind" default:"figma-map.binding.yaml"`
	Catalog string `json:"catalog" jsonschema:"catalog directory from scan" default:"catalog"`
}

var planOp = Op[planIn, service.Plan]{
	Name:    "plan",
	Summary: "Map every component instance in a Figma frame to code (buildable spec)",
	Run: func(ctx context.Context, s *service.Service, in planIn) (service.Plan, error) {
		return s.Plan(ctx, in.File, in.FrameID, in.Depth, in.Binding, in.Catalog)
	},
	Render: func(p service.Plan) string {
		var b strings.Builder
		fmt.Fprintf(&b, "%s (%.0f×%.0f)", p.Frame.Name, p.Frame.Width, p.Frame.Height)
		if p.Layout != nil {
			fmt.Fprintf(&b, " — layout: %s", p.Layout.Direction)
			if p.Layout.Gap != nil {
				fmt.Fprintf(&b, " gap %.0f", *p.Layout.Gap)
			}
		}
		b.WriteString("\n")
		for _, c := range p.Components {
			fmt.Fprintf(&b, "  ✓ %s (%s) %v %.2f\n", c.Component, c.Import, c.Props, c.Confidence)
		}
		for _, u := range p.Unmapped {
			fmt.Fprintf(&b, "  – %s [%s] — %s\n", u.Name, u.Type, u.Reason)
		}
		return strings.TrimRight(b.String(), "\n")
	},
}

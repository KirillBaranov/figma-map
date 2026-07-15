package op

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/render"
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
		bridgeUpOp,
		bridgeDownOp,
		bridgeStatusOp,
		scanOp,
		bindOp,
		listOp,
		tokensOp,
		animationOp,
		variablesOp,
		inspectOp,
		selectionOp,
		pagesOp,
		screenshotOp,
		browserScreenshotOp,
		renderOp,
		exportAssetsOp,
		mapOp,
		planOp,
		findOp,
		codegenOp,
		reconcileOp,
		pixelDiffOp,
		pixelDiffImagesOp,
		captureIssuesOp,
		captureAckOp,
	}
}

// ---- doctor ----

type doctorIn struct{}

var doctorOp = Op[doctorIn, service.Report]{
	Group:   "",
	Verb:    "doctor",
	Summary: "Check that the bridge, Chrome, Storybook, and API key are available",
	Run: func(ctx context.Context, s *service.Service, _ doctorIn) (service.Report, error) {
		return s.Doctor(ctx), nil
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

// ---- bridge up ----

type bridgeUpIn struct {
	Repo string `json:"repo,omitempty" jsonschema:"figma-map source checkout path (default: bridgeRepo in figma-map.yaml)"`
}

var bridgeUpOp = Op[bridgeUpIn, service.BridgeUpResult]{
	Group:   "bridge",
	Verb:    "up",
	Summary: "Start the local backend if nothing is already listening on the configured bridge URL",
	Long: "up pings the configured bridge URL first and does nothing if something's already " +
		"there. Otherwise it builds backend/dist/index.js if missing (npm --prefix backend run " +
		"build) and starts it detached from --repo (or the bridgeRepo config field), so it " +
		"outlives this command. Never starts a second copy.",
	Run: func(ctx context.Context, s *service.Service, in bridgeUpIn) (service.BridgeUpResult, error) {
		return s.BridgeUp(ctx, in.Repo)
	},
	Render: func(r service.BridgeUpResult) string {
		if r.AlreadyRunning {
			return fmt.Sprintf("already running: %s", r.Bridge)
		}
		return fmt.Sprintf("started backend (pid %d), answering %s — log: %s", r.PID, r.Bridge, r.LogPath)
	},
}

// ---- bridge down ----

type bridgeDownIn struct{}

var bridgeDownOp = Op[bridgeDownIn, service.BridgeDownResult]{
	Group:   "bridge",
	Verb:    "down",
	Summary: "Stop the backend process started by a prior `bridge up`",
	Run: func(ctx context.Context, s *service.Service, _ bridgeDownIn) (service.BridgeDownResult, error) {
		return s.BridgeDown(ctx)
	},
	Render: func(r service.BridgeDownResult) string {
		if !r.Stopped {
			return r.Reason
		}
		return fmt.Sprintf("stopped pid %d", r.PID)
	},
}

// ---- bridge status ----

type bridgeStatusIn struct{}

var bridgeStatusOp = Op[bridgeStatusIn, service.BridgeStatusResult]{
	Group:   "bridge",
	Verb:    "status",
	Summary: "Check whether the backend is reachable, and the pid/log `bridge up` recorded for it",
	Run: func(ctx context.Context, s *service.Service, _ bridgeStatusIn) (service.BridgeStatusResult, error) {
		return s.BridgeStatus(ctx)
	},
	Render: func(r service.BridgeStatusResult) string {
		state := "not reachable"
		if r.Running {
			state = "reachable"
		}
		s := fmt.Sprintf("%s: %s", r.Bridge, state)
		if r.PID != 0 {
			s += fmt.Sprintf(" (pid %d)", r.PID)
		}
		if r.LogPath != "" {
			s += fmt.Sprintf(" — log: %s", r.LogPath)
		}
		return s
	},
}

// ---- scan ----

type scanIn struct {
	Storybook string `json:"storybook,omitempty" jsonschema:"Storybook base URL (default from config)"`
	Project   string `json:"project,omitempty" jsonschema:"project root with *.stories source files" default:"."`
	Out       string `json:"out,omitempty" jsonschema:"output catalog directory" default:"catalog"`
}

var scanOp = Op[scanIn, service.ScanResult]{
	Group:   "setup",
	Verb:    "scan",
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
	File    string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
	Catalog string `json:"catalog,omitempty" jsonschema:"catalog directory from scan" default:"catalog"`
	Out     string `json:"out,omitempty" jsonschema:"output binding file" default:"figma-map.binding.yaml"`
}

var bindOp = Op[bindIn, service.BindResult]{
	Group:   "setup",
	Verb:    "bind",
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
	Binding string `json:"binding,omitempty" jsonschema:"binding file from bind" default:"figma-map.binding.yaml"`
}

var listOp = Op[listIn, service.ListResult]{
	Group:   "setup",
	Verb:    "components",
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
	File    string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
	Binding string `json:"binding,omitempty" jsonschema:"binding file from bind" default:"figma-map.binding.yaml"`
	Catalog string `json:"catalog,omitempty" jsonschema:"catalog directory from scan" default:"catalog"`
}

var mapOp = Op[mapIn, service.MapResult]{
	Group:   "build",
	Verb:    "map",
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
	File   string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
}

var tokensOp = Op[tokensIn, service.TokensResult]{
	Group:   "figma",
	Verb:    "tokens",
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

// ---- animation ----

type animationIn struct {
	NodeID string `json:"nodeId" jsonschema:"Figma node id" cli:"arg"`
	File   string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
}

var animationOp = Op[animationIn, service.AnimationResult]{
	Group:   "figma",
	Verb:    "animation",
	Summary: "Resolve a node's prototyping reactions to actual before/after style deltas",
	Long: "figma tokens' reactions field is cheap trigger/timing data (trigger, transitionType, " +
		"easing, duration), carried for every node in a tree walk. `figma animation` does the " +
		"expensive part on top: resolving what actually changes. When a reaction navigates to " +
		"another node (a real destination the designer set), it diffs that node's styles against " +
		"this one's. When it doesn't (a same-component hover/press/focus state with no explicit " +
		"destination), it guesses a sibling variant from the component set and says so via " +
		"resolvedVia=\"variant-sibling\" rather than presenting a guess as ground truth. Use " +
		"styleDelta's from/to to write a real CSS transition or framer-motion animate prop, not " +
		"just a note that something animates.",
	Run: func(ctx context.Context, s *service.Service, in animationIn) (service.AnimationResult, error) {
		return s.GetAnimation(ctx, in.File, in.NodeID)
	},
	Render: func(r service.AnimationResult) string {
		if len(r.Animations) == 0 {
			return fmt.Sprintf("%s: no reactions", r.Name)
		}
		return fmt.Sprintf("%s:\n%s", r.Name, indentJSON(r.Animations))
	},
}

// ---- variables ----

type variablesIn struct {
	File string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
}

var variablesOp = Op[variablesIn, service.VariablesResult]{
	Group:   "figma",
	Verb:    "variables",
	Summary: "List every Figma Variable defined in the file (the token catalog, not per-node bindings)",
	Long: "variables returns the file's full local-variable catalog — every collection, " +
		"every variable, every mode's value — independent of any specific node. " +
		"Use it to see what tokens exist in the file; use `tokens` on a specific node " +
		"to see which of these (if any) that node is actually bound to.",
	Run: func(ctx context.Context, s *service.Service, in variablesIn) (service.VariablesResult, error) {
		return s.Variables(ctx, in.File)
	},
	Render: func(r service.VariablesResult) string {
		if len(r.Collections) == 0 {
			return "no variable collections in this file"
		}
		var b strings.Builder
		for _, c := range r.Collections {
			fmt.Fprintf(&b, "%s (%d mode(s)):\n", c.Name, len(c.Modes))
			for _, v := range c.Variables {
				fmt.Fprintf(&b, "  %-30s %s\n", v.Name, v.ResolvedType)
			}
		}
		return strings.TrimRight(b.String(), "\n")
	},
}

// ---- pages ----

type pagesIn struct {
	File string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
}

var pagesOp = Op[pagesIn, service.PagesResult]{
	Group:   "figma",
	Verb:    "pages",
	Summary: "List the file's pages — the discovery entry point when you don't have a node id yet",
	Long: "pages returns the file name and page list only — no tree, no styles. " +
		"Use it first to get oriented in a file, then `find`/`inspect` to drill into a " +
		"specific page or frame.",
	Run: func(ctx context.Context, s *service.Service, in pagesIn) (service.PagesResult, error) {
		return s.Pages(ctx, in.File)
	},
	Render: func(r service.PagesResult) string {
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n", r.FileName)
		for _, p := range r.Pages {
			mark := "  "
			if p.ID == r.CurrentPageID {
				mark = "→ "
			}
			fmt.Fprintf(&b, "%s%-20s  %s\n", mark, p.ID, p.Name)
		}
		return strings.TrimRight(b.String(), "\n")
	},
}

// ---- inspect ----

type inspectIn struct {
	NodeID string `json:"nodeId" jsonschema:"Figma node id" cli:"arg"`
	File   string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
	Tokens bool   `json:"tokens,omitempty" jsonschema:"include design tokens per node"`
	Depth  int    `json:"depth,omitempty" jsonschema:"max tree depth (0 = unlimited)" default:"0"`
}

var inspectOp = Op[inspectIn, service.InspectResult]{
	Group:   "figma",
	Verb:    "inspect",
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

// ---- selection ----

type selectionIn struct {
	File string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
}

var selectionOp = Op[selectionIn, service.SelectionResult]{
	Group:   "figma",
	Verb:    "selection",
	Summary: "Get the node(s) currently selected in the Figma editor",
	Long: "selection asks the running Figma plugin which node(s) the user has selected right now. " +
		"Use it to act on \"the selected layer\" without the user having to copy a node id — " +
		"pipe the returned id into tokens, inspect, screenshot, codegen, or pixeldiff.",
	Run: func(ctx context.Context, s *service.Service, in selectionIn) (service.SelectionResult, error) {
		return s.Selection(ctx, in.File)
	},
	Render: func(r service.SelectionResult) string {
		if len(r.Nodes) == 0 {
			return "no selection — select a layer in Figma and try again"
		}
		var b strings.Builder
		for _, n := range r.Nodes {
			fmt.Fprintf(&b, "%-20s  %-18s  %s\n", n.ID, n.Type, n.Name)
		}
		return strings.TrimRight(b.String(), "\n")
	},
}

// ---- render ----

type renderIn struct {
	NodeID string  `json:"nodeId" jsonschema:"Figma node id" cli:"arg"`
	File   string  `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
	Out    string  `json:"out,omitempty" jsonschema:"output PNG path; default: .figma-map/out/<nodeId>-render.png"`
	Scale  float64 `json:"scale,omitempty" jsonschema:"export scale factor" default:"1"`
	Inline bool    `json:"inline,omitempty" jsonschema:"also return the PNG bytes inline (MCP) instead of just the path"`
}

var renderOp = Op[renderIn, service.RenderResult]{
	Group:   "capture",
	Verb:    "render",
	Summary: "Screenshot figma-map's own raw codegen output for a node — no app or server needed",
	Long: "render generates standalone HTML directly from the node's Figma tree (the same CSS " +
		"codegen uses, but plain style=\"\" attributes and no UIKit component substitution, " +
		"since there's no app to mount one in) and screenshots it headless via a file:// URL. " +
		"Use it to sanity-check the converter's CSS before writing any real implementation, or " +
		"pass no --url to `pixeldiff` to diff against this directly. Always writes a PNG to " +
		"--out (or a default .figma-map/out/ path) — pass --inline to also get the bytes back.",
	Run: func(ctx context.Context, s *service.Service, in renderIn) (service.RenderResult, error) {
		return s.Render(ctx, in.File, in.NodeID, in.Scale, in.Out, in.Inline)
	},
	Render: func(r service.RenderResult) string {
		return fmt.Sprintf("wrote %s (%d×%d)", r.Path, r.Width, r.Height)
	},
	Image: func(r service.RenderResult) ([]byte, string) {
		return r.PNG, "image/png"
	},
}

// ---- screenshot ----

type screenshotIn struct {
	NodeID string  `json:"nodeId" jsonschema:"Figma node id" cli:"arg"`
	File   string  `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
	Out    string  `json:"out,omitempty" jsonschema:"output PNG path; default: .figma-map/out/<nodeId>-screenshot.png"`
	Scale  float64 `json:"scale,omitempty" jsonschema:"export scale factor" default:"2"`
	Inline bool    `json:"inline,omitempty" jsonschema:"also return the PNG bytes inline (MCP) instead of just the path"`
}

var screenshotOp = Op[screenshotIn, service.ScreenshotResult]{
	Group:   "capture",
	Verb:    "screenshot",
	Summary: "Render a Figma node to a PNG image",
	Long: "Always writes a PNG to --out (or a default .figma-map/out/ path) and returns the " +
		"path — pass --inline to also get the bytes back (MCP), for the rare case a vision " +
		"model needs to see the image directly in the response.",
	Run: func(ctx context.Context, s *service.Service, in screenshotIn) (service.ScreenshotResult, error) {
		return s.Screenshot(ctx, in.File, in.NodeID, in.Scale, in.Out, in.Inline)
	},
	Render: func(r service.ScreenshotResult) string {
		return fmt.Sprintf("wrote %s (%d×%d)", r.Path, r.Width, r.Height)
	},
	Image: func(r service.ScreenshotResult) ([]byte, string) {
		return r.PNG, "image/png"
	},
}

// ---- browser screenshot ----

type browserScreenshotIn struct {
	URL      string  `json:"url" jsonschema:"http(s):// URL, or a local HTML file path" cli:"arg"`
	Selector string  `json:"selector,omitempty" jsonschema:"CSS selector (or a bare Figma node id, expanded to [data-figma-node=\"<id>\"]) to crop to one element instead of the whole viewport"`
	Width    int     `json:"width,omitempty" jsonschema:"browser viewport width in CSS px (default 1280)"`
	Scale    float64 `json:"scale,omitempty" jsonschema:"deviceScaleFactor" default:"1"`
	Out      string  `json:"out,omitempty" jsonschema:"output PNG path; default: .figma-map/out/<selector-or-page>-browser-screenshot.png"`
	Inline   bool    `json:"inline,omitempty" jsonschema:"also return the PNG bytes inline (MCP) instead of just the path"`
}

var browserScreenshotOp = Op[browserScreenshotIn, service.BrowserScreenshotResult]{
	Group:   "capture",
	Verb:    "browser",
	Summary: "Screenshot a live URL, optionally cropped to one element",
	Long: "Screenshots url (a dev server, Storybook iframe, or local HTML file) the same way " +
		"`verify pixeldiff --selector` does internally, but standalone — for looking at what's " +
		"currently rendered without comparing it against a Figma node. Without --selector, " +
		"captures the whole viewport; with it, crops to the matching element (CSS selector or a " +
		"bare Figma node id, expanded to [data-figma-node=\"<id>\"]) inside a normally-sized page " +
		"viewport, so a mid-page section can be checked without setting up isolation for it. " +
		"Always writes a PNG to --out (or a default .figma-map/out/ path) — pass --inline to " +
		"also get the bytes back.",
	Run: func(ctx context.Context, s *service.Service, in browserScreenshotIn) (service.BrowserScreenshotResult, error) {
		return s.BrowserScreenshot(ctx, in.URL, in.Selector, in.Width, in.Scale, in.Out, in.Inline)
	},
	Render: func(r service.BrowserScreenshotResult) string {
		return fmt.Sprintf("wrote %s (%d×%d)", r.Path, r.Width, r.Height)
	},
	Image: func(r service.BrowserScreenshotResult) ([]byte, string) {
		return r.PNG, "image/png"
	},
}

// ---- export-assets ----

type exportIn struct {
	NodeID string `json:"nodeId" jsonschema:"Figma node id to export" cli:"arg"`
	File   string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
	Format string `json:"format,omitempty" jsonschema:"PNG, SVG, or JPG" default:"SVG"`
	Out    string `json:"out,omitempty" jsonschema:"output directory" default:"assets"`
}

var exportAssetsOp = Op[exportIn, service.ExportResult]{
	Group:   "capture",
	Verb:    "export",
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
	NodeID  string `json:"nodeId" jsonschema:"Figma frame node id to map" cli:"arg"`
	File    string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
	Depth   int    `json:"depth,omitempty" jsonschema:"max nesting depth to search (0 = unlimited)" default:"0"`
	Binding string `json:"binding,omitempty" jsonschema:"binding file from bind" default:"figma-map.binding.yaml"`
	Catalog string `json:"catalog,omitempty" jsonschema:"catalog directory from scan" default:"catalog"`
}

var planOp = Op[planIn, service.Plan]{
	Group:   "build",
	Verb:    "plan",
	Summary: "Map every component instance in a Figma frame to code (buildable spec)",
	Run: func(ctx context.Context, s *service.Service, in planIn) (service.Plan, error) {
		return s.Plan(ctx, in.File, in.NodeID, in.Depth, in.Binding, in.Catalog)
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

// ---- codegen ----

type codegenIn struct {
	NodeID  string `json:"nodeId" jsonschema:"Figma node id of the frame to generate code for" cli:"arg"`
	File    string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
	Binding string `json:"binding,omitempty" jsonschema:"binding file for UIKit component mapping" default:"figma-map.binding.yaml"`
	Target  string `json:"target,omitempty" jsonschema:"Output renderer: jsx (default) or html. Falls back to figma-map.yaml's codegen.target, then jsx."`
}

var codegenOp = Op[codegenIn, service.CodegenResult]{
	Group:   "build",
	Verb:    "codegen",
	Summary: "Generate a component from a Figma frame (full tree — layout, text, UIKit components)",
	Long: "codegen walks the entire Figma node tree and emits a ready-to-edit component file. " +
		"Every FRAME becomes a <div> with flex/grid styles from auto-layout, " +
		"every TEXT node becomes a <span>/<p> with typography styles, " +
		"and every INSTANCE matched in the binding becomes its UIKit component. " +
		"Pass a section-level frame (not the whole page) for best results. " +
		"--target selects the output renderer (jsx by default); more targets are added over time.",
	Run: func(ctx context.Context, s *service.Service, in codegenIn) (service.CodegenResult, error) {
		return s.Codegen(ctx, in.File, in.NodeID, in.Binding, in.Target)
	},
	Render: func(r service.CodegenResult) string {
		return r.Code
	},
}

// ---- reconcile ----

type reconcileIn struct {
	NodeID   string `json:"nodeId" jsonschema:"Figma node id to reconcile against" cli:"arg"`
	File     string `json:"file,omitempty" jsonschema:"Figma file key (default: config or sole connected file)"`
	Story    string `json:"story,omitempty" jsonschema:"Storybook story id rendering the implementation"`
	URL      string `json:"url,omitempty" jsonschema:"URL rendering the implementation (alternative to story)"`
	Image    string `json:"image,omitempty" jsonschema:"flat image path (no-DOM fallback, Tier 2 only)"`
	Semantic bool   `json:"semantic,omitempty" jsonschema:"also run the Tier-2 semantic LLM check"`
}

var reconcileOp = Op[reconcileIn, service.Diff]{
	Group:   "verify",
	Verb:    "reconcile",
	Summary: "Compare a Figma node against rendered output (deterministic token diff)",
	Long: "reconcile renders the implementation (story or url), reads its DOM " +
		"computed styles, and diffs them against the Figma node's exact tokens — " +
		"returning per-element is/should numbers, not a vision guess. Elements must " +
		"carry data-figma-node=\"<id>\" to be measured.",
	Run: func(ctx context.Context, s *service.Service, in reconcileIn) (service.Diff, error) {
		return s.Reconcile(ctx, in.File, in.NodeID, in.Story, in.URL, in.Image, in.Semantic)
	},
	Render: renderReconcile,
}

// renderReconcile formats a reconcile Diff as a human-readable report — the
// thing an agent hands to a person when it can't fully converge.
func renderReconcile(d service.Diff) string {
	var b strings.Builder

	// Headline.
	if d.Match {
		fmt.Fprintf(&b, "✓ match within tolerance (%d/%d nodes measured)\n",
			d.Coverage.Measured, d.Coverage.Targets)
	} else {
		fmt.Fprintf(&b, "✗ %d fixable difference(s)", d.Remaining)
		if d.Advisory > 0 {
			fmt.Fprintf(&b, " · %d advisory", d.Advisory)
		}
		fmt.Fprintf(&b, " · %d/%d nodes measured\n", d.Coverage.Measured, d.Coverage.Targets)
	}

	// Fixable diffs first (the agent should resolve these).
	fixable := elementsWith(d.ByElement, false)
	if len(fixable) > 0 {
		b.WriteString("\nFix these:\n")
		for _, e := range fixable {
			fmt.Fprintf(&b, "  %s (%s):\n", e.Name, e.NodeID)
			for _, f := range e.Diffs {
				if !f.Advisory {
					fmt.Fprintf(&b, "    %s: %s → should be %s\n", f.Prop, f.Is, f.Should)
				}
			}
		}
	}

	// Advisory diffs (may be content-driven — judge, don't blindly change).
	advisory := elementsWith(d.ByElement, true)
	if len(advisory) > 0 {
		b.WriteString("\nAdvisory (may be content-driven):\n")
		for _, e := range advisory {
			for _, f := range e.Diffs {
				if f.Advisory {
					fmt.Fprintf(&b, "  %s (%s) %s: %s → %s\n", e.Name, e.NodeID, f.Prop, f.Is, f.Should)
				}
			}
		}
	}

	// Semantic findings.
	if len(d.Semantic) > 0 {
		b.WriteString("\nSemantic:\n")
		for _, f := range d.Semantic {
			fmt.Fprintf(&b, "  [%s/%s] %s\n", f.Kind, f.Severity, f.Detail)
		}
	}

	// Unmeasured, split into actionable vs expected.
	var toTag []service.UnmeasuredNode
	decorative := 0
	for _, u := range d.Unmeasured {
		if u.Actionable {
			toTag = append(toTag, u)
		} else {
			decorative++
		}
	}
	if len(toTag) > 0 {
		b.WriteString("\nNot verified — tag these with data-figma-node:\n")
		for _, u := range toTag {
			fmt.Fprintf(&b, "  - %s [%s] %s\n", u.Name, u.Type, u.NodeID)
		}
	}
	if decorative > 0 {
		fmt.Fprintf(&b, "\n%d decorative/image node(s) not DOM-measurable (expected).\n", decorative)
	}
	if len(d.SpatiallyAligned) > 0 {
		fmt.Fprintf(&b, "\n%d node(s) matched by position, not by tag (lower confidence).\n",
			len(d.SpatiallyAligned))
	}

	return strings.TrimRight(b.String(), "\n")
}

func elementsWith(els []service.ElementDiff, advisory bool) []service.ElementDiff {
	var out []service.ElementDiff
	for _, e := range els {
		for _, f := range e.Diffs {
			if f.Advisory == advisory {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// --- pixeldiff ---

type pixelDiffIn struct {
	NodeID    string  `json:"nodeId"    jsonschema:"Figma node id of the frame to compare" cli:"arg"`
	URL       string  `json:"url,omitempty"       jsonschema:"rendered implementation to compare against: http(s):// URL, a local HTML file path, or omit to diff against figma-map's own raw codegen render"`
	File      string  `json:"file,omitempty"      jsonschema:"Figma file key (default: config or sole connected file)"`
	Threshold float64 `json:"threshold,omitempty" jsonschema:"max diff% before match=false (default 5)" default:"5"`
	ColorTol  int     `json:"colorTol,omitempty"  jsonschema:"per-channel color tolerance 0-255 (default 10)" default:"10"`
	DiffOut   string  `json:"diffOut,omitempty"   jsonschema:"path to write annotated diff PNG (optional)"`
	GridSize  int     `json:"gridSize,omitempty"  jsonschema:"break the diff into an NxN grid of per-cell diff% (default 4; negative disables)" default:"4"`
	Selector  string  `json:"selector,omitempty"  jsonschema:"CSS selector (or a bare Figma node id, expanded to [data-figma-node=\"<id>\"]) scoping the implementation screenshot to one element instead of the whole viewport — use this to diff a section that lives mid-page, not rendered as its own isolated story/HTML"`
	Width     int     `json:"width,omitempty"     jsonschema:"browser viewport width in CSS px, only used with --selector (default 1280 — a scoped section's layout is usually driven by its page/container width, not its own size)"`
}

var pixelDiffOp = Op[pixelDiffIn, service.PixelDiffResult]{
	Group:   "verify",
	Verb:    "pixeldiff",
	Summary: "Pixel-level screenshot comparison: Figma design vs browser implementation",
	Long: "pixeldiff takes a screenshot of the Figma node and a screenshot of the implementation " +
		"(viewport sized to the node), then compares them pixel-by-pixel. " +
		"Returns diffPct and match=true when diffPct ≤ threshold, plus a gridSize×gridSize " +
		"Regions breakdown (per-cell diff%, worst first) — read that instead of the diff image " +
		"to find where the mismatch is without having to visually interpret an overlay. " +
		"--url accepts an http(s):// URL for an already-running implementation (dev server, " +
		"Storybook iframe — should render the component in isolation so both images cover the " +
		"same region), or a local HTML file path (screenshotted directly, no server needed). " +
		"Omit --url entirely to diff against figma-map's own raw codegen render (see `render`) — " +
		"useful before there's a real implementation to point at. " +
		"--selector scopes the implementation screenshot to one element (CSS selector or bare " +
		"Figma node id) instead of the whole viewport, so a section that lives mid-page can be " +
		"diffed without setting up isolation for it first.",
	Run: func(ctx context.Context, s *service.Service, in pixelDiffIn) (service.PixelDiffResult, error) {
		return s.PixelDiff(ctx, in.File, in.NodeID, in.URL, service.PixelDiffOptions{
			Threshold: in.Threshold,
			ColorTol:  uint8(in.ColorTol),
			DiffOut:   in.DiffOut,
			GridSize:  in.GridSize,
			Scale:     1,
			Selector:  in.Selector,
			Width:     in.Width,
		})
	},
	Render: func(r service.PixelDiffResult) string {
		icon := "✓"
		if !r.Match {
			icon = "✗"
		}
		s := fmt.Sprintf("%s pixel diff: %.2f%% (threshold %.0f%%)\n  %d / %d pixels differ",
			icon, r.DiffPct, r.Threshold, r.DiffPixels, r.Total)
		if n := len(r.Regions); n > 0 {
			top := r.Regions
			if n > 3 {
				top = top[:3]
			}
			s += "\n  worst regions:"
			for _, reg := range top {
				s += fmt.Sprintf("\n    (%d,%d,%d×%d): %.2f%%", reg.X, reg.Y, reg.W, reg.H, reg.DiffPct)
			}
		}
		if r.DiffOut != "" {
			s += fmt.Sprintf("\n  diff image → %s", r.DiffOut)
		}
		return s
	},
}

// --- pixeldiff-images ---

type pixelDiffImagesIn struct {
	Image1    string  `json:"image1"    jsonschema:"first image: a local file path, a data:image/...;base64,... URI, or a bare base64 PNG string" cli:"arg"`
	Image2    string  `json:"image2"    jsonschema:"second image: a local file path, a data:image/...;base64,... URI, or a bare base64 PNG string" cli:"arg"`
	Threshold float64 `json:"threshold,omitempty" jsonschema:"max diff% before match=false (default 5)" default:"5"`
	ColorTol  int     `json:"colorTol,omitempty"  jsonschema:"per-channel color tolerance 0-255 (default 10)" default:"10"`
	DiffOut   string  `json:"diffOut,omitempty"   jsonschema:"path to write annotated diff PNG (optional)"`
	GridSize  int     `json:"gridSize,omitempty"  jsonschema:"break the diff into an NxN grid of per-cell diff% (default 4; negative disables)" default:"4"`
}

var pixelDiffImagesOp = Op[pixelDiffImagesIn, service.PixelDiffResult]{
	Group:   "verify",
	Verb:    "pixeldiff-images",
	Summary: "Pixel-level comparison between two already-captured images, no Figma or browser fetch",
	Long: "pixeldiff-images compares two raw images directly, with no Figma node lookup and no " +
		"browser render — for when both sides were already captured elsewhere (e.g. a browser " +
		"extension grabbing a live-page crop) and there's no URL or node id to re-fetch from. " +
		"Each image argument may be a local file path, a data:image/...;base64,... URI, or a " +
		"bare base64 PNG string. Returns the same diffPct/match/Regions shape as `verify pixeldiff`.",
	Run: func(_ context.Context, _ *service.Service, in pixelDiffImagesIn) (service.PixelDiffResult, error) {
		img1, err := loadImageInput(in.Image1)
		if err != nil {
			return service.PixelDiffResult{}, fmt.Errorf("image1: %w", err)
		}
		img2, err := loadImageInput(in.Image2)
		if err != nil {
			return service.PixelDiffResult{}, fmt.Errorf("image2: %w", err)
		}

		threshold := in.Threshold
		if threshold <= 0 {
			threshold = 5
		}
		colorTol := uint8(in.ColorTol)
		if in.ColorTol == 0 {
			colorTol = 10
		}
		gridSize := in.GridSize
		if gridSize == 0 {
			gridSize = 4
		} else if gridSize < 0 {
			gridSize = 0
		}

		diff, err := render.PixelDiff(img1, img2, colorTol, in.DiffOut != "", gridSize)
		if err != nil {
			return service.PixelDiffResult{}, fmt.Errorf("pixel diff: %w", err)
		}

		result := service.PixelDiffResult{
			Match:      diff.DiffPct <= threshold,
			DiffPct:    math.Round(diff.DiffPct*100) / 100,
			DiffPixels: diff.DiffPixels,
			Total:      diff.TotalPixels,
			Threshold:  threshold,
		}
		for _, r := range diff.Regions {
			result.Regions = append(result.Regions, service.DiffRegion{
				X: r.X, Y: r.Y, W: r.W, H: r.H,
				DiffPct: math.Round(r.DiffPct*100) / 100,
			})
		}

		if in.DiffOut != "" && len(diff.DiffImage) > 0 {
			if err := os.WriteFile(in.DiffOut, diff.DiffImage, 0o644); err != nil {
				return result, fmt.Errorf("write diff image: %w", err)
			}
			result.DiffOut = in.DiffOut
		}

		return result, nil
	},
	Render: pixelDiffOp.Render,
}

// loadImageInput resolves an image argument that may be a local file path, a
// data:image/...;base64,... URI, or a bare base64-encoded PNG string.
func loadImageInput(input string) ([]byte, error) {
	if input == "" {
		return nil, fmt.Errorf("empty image input")
	}
	if strings.HasPrefix(input, "data:") {
		if idx := strings.Index(input, ";base64,"); idx != -1 {
			return base64.StdEncoding.DecodeString(input[idx+len(";base64,"):])
		}
		return nil, fmt.Errorf("data URI missing ;base64, marker")
	}
	if data, err := os.ReadFile(input); err == nil {
		return data, nil
	}
	return base64.StdEncoding.DecodeString(input)
}

// --- capture issues / ack ---

type captureIssuesIn struct {
	File string `json:"file,omitempty" jsonschema:"Figma file key to filter by (default: all)"`
}

var captureIssuesOp = Op[captureIssuesIn, service.IssuesResult]{
	Group:   "capture",
	Verb:    "issues",
	Summary: "List flagged issues reported by the browser extension",
	Long: "issues lists regions of a live page a human flagged via the browser extension " +
		"(screenshot, bounds, CSS selector, optional linked Figma node id, optional note). " +
		"Pair a linked figmaNodeId with `figma screenshot` and `verify pixeldiff-images` to get " +
		"a structured diff, then call `capture ack` once handled.",
	Run: func(ctx context.Context, s *service.Service, in captureIssuesIn) (service.IssuesResult, error) {
		return s.ListIssues(ctx, in.File)
	},
	Render: func(r service.IssuesResult) string {
		if len(r.Issues) == 0 {
			return "no flagged issues"
		}
		var b strings.Builder
		for _, iss := range r.Issues {
			fmt.Fprintf(&b, "%s  %s", iss.ID, iss.Selector)
			if iss.FigmaNodeID != "" {
				fmt.Fprintf(&b, "  -> %s", iss.FigmaNodeID)
			}
			if iss.RegionNodeID != "" {
				fmt.Fprintf(&b, "  region:%s", iss.RegionNodeID)
			}
			if iss.Note != "" {
				fmt.Fprintf(&b, "  %q", iss.Note)
			}
			b.WriteString("\n")
		}
		return strings.TrimRight(b.String(), "\n")
	},
}

type captureAckIn struct {
	ID string `json:"id" jsonschema:"id of the flagged issue to mark handled" cli:"arg"`
}

var captureAckOp = Op[captureAckIn, service.AckResult]{
	Group:   "capture",
	Verb:    "ack",
	Summary: "Mark a flagged issue as handled, removing it from the inbox",
	Run: func(ctx context.Context, s *service.Service, in captureAckIn) (service.AckResult, error) {
		return s.AckIssue(ctx, in.ID)
	},
	Render: func(r service.AckResult) string {
		return fmt.Sprintf("acked %s", r.ID)
	},
}

// --- find ---

type findIn struct {
	Query      string `json:"query"      jsonschema:"text to search in node names (case-insensitive)" cli:"arg"`
	Text       string `json:"text,omitempty"       jsonschema:"filter: node must contain this text (TEXT nodes)"`
	Type       string `json:"type,omitempty"       jsonschema:"filter: Figma node type, e.g. FRAME, TEXT, INSTANCE"`
	Mode       string `json:"mode,omitempty"       jsonschema:"filter: variable mode override value, e.g. Dark, Light, Compact"`
	Within     string `json:"within,omitempty"     jsonschema:"restrict search to a specific node subtree (node id)"`
	Depth      int    `json:"depth,omitempty"      jsonschema:"max nesting depth to fetch within --within's subtree (0 = unlimited); lower this if a large section times out"`
	File       string `json:"file,omitempty"       jsonschema:"Figma file key (default: config or sole connected file)"`
	MaxResults int    `json:"maxResults,omitempty" jsonschema:"max nodes to return (default 50)" default:"50"`
}

var findOp = Op[findIn, service.FindResults]{
	Group:   "figma",
	Verb:    "find",
	Summary: "Search Figma nodes by name (and optionally text content or type)",
	Long: "find walks the entire Figma document tree and returns nodes whose name contains " +
		"the query string. Use --text to additionally filter by text content, --type to " +
		"filter by Figma node type (FRAME, TEXT, INSTANCE, COMPONENT, …). " +
		"Results include node ID, name, type, and breadcrumb path — pipe the ID into codegen or pixeldiff.",
	Run: func(ctx context.Context, s *service.Service, in findIn) (service.FindResults, error) {
		return s.Find(ctx, in.File, service.FindOptions{
			Query:        in.Query,
			TextQuery:    in.Text,
			NodeType:     in.Type,
			Mode:         in.Mode,
			WithinNodeID: in.Within,
			Depth:        in.Depth,
			MaxResults:   in.MaxResults,
		})
	},
	Render: func(r service.FindResults) string {
		results := r.Nodes
		if len(results) == 0 {
			return "no nodes found"
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "found %d node(s):\n", len(results))
		for _, r := range results {
			line := fmt.Sprintf("  %-20s  %-18s  %s", r.ID, r.Type, r.Name)
			if r.Path != "" {
				line += "\n" + fmt.Sprintf("  %-20s  %-18s  ↳ %s", "", "", r.Path)
			}
			if len(r.VariantModes) > 0 {
				modes := make([]string, 0, len(r.VariantModes))
				for k, v := range r.VariantModes {
					modes = append(modes, k+": "+v)
				}
				sort.Strings(modes)
				line += "\n" + fmt.Sprintf("  %-20s  %-18s  ⬡ %s", "", "", strings.Join(modes, ", "))
			}
			if len(r.ComponentProps) > 0 {
				props := make([]string, 0, len(r.ComponentProps))
				for k, v := range r.ComponentProps {
					props = append(props, fmt.Sprintf("%s=%v", k, v))
				}
				sort.Strings(props)
				line += "\n" + fmt.Sprintf("  %-20s  %-18s  ◈ %s", "", "", strings.Join(props, ", "))
			}
			if r.Text != "" {
				short := r.Text
				if len(short) > 60 {
					short = short[:60] + "…"
				}
				line += "\n" + fmt.Sprintf("  %-20s  %-18s    \"%s\"", "", "", short)
			}
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
		return sb.String()
	},
}

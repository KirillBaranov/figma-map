// Package figma defines the domain model for a Figma document and the Source
// interface that abstracts where that data comes from. The bridge backend
// (figma-mcp-bridge) is the only implementation today; a REST backend can be
// added later behind the same interface.
package figma

import "context"

// Bounds is the absolute bounding box of a node, in Figma canvas coordinates.
type Bounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Node is the source-independent representation of a Figma node. It carries
// only the fields figma-map needs; richer per-backend data is intentionally
// dropped at the boundary so downstream code stays backend-agnostic.
type Node struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Characters string `json:"characters,omitempty"`
	Bounds     Bounds `json:"bounds"`
	Styles     *Style `json:"styles,omitempty"`
	Children   []Node `json:"children,omitempty"`
	// ComponentProps holds variant/property values for INSTANCE nodes,
	// e.g. {"State": "Hover", "Size": "M", "hasIcon": true}.
	ComponentProps map[string]any `json:"componentProps,omitempty"`
	// VariantModes holds explicit variable mode overrides resolved to names,
	// e.g. {"Color Semantic": "Dark"} — the source of the badges in Figma's layers panel.
	VariantModes map[string]string `json:"variantModes,omitempty"`
	// GridPosition is this node's explicit row/column placement within its
	// parent's GRID auto-layout, if the parent uses one.
	GridPosition *GridPosition `json:"gridPosition,omitempty"`
	// Reactions are this node's prototyping reactions (click/hover →
	// navigate/animate), if any are configured. Ground truth for
	// interactive-state timing — ground truth in, never auto-applied as CSS;
	// whether a reaction should become a real transition is a judgment call.
	Reactions []Reaction `json:"reactions,omitempty"`
	// DevStatus is "READY_FOR_DEV" or "COMPLETED" — only settable on a node
	// directly under a page or section. A discovery filter, not a guess.
	DevStatus string `json:"devStatus,omitempty"`
	// DevResources are designer-attached links (name+url) — a node-level
	// analogue of a Variable's CodeSyntax.
	DevResources []DevResource `json:"devResources,omitempty"`
	// Annotations are designer notes/instructions on this node, free text —
	// never auto-applied, but a strong human-given hint to read.
	Annotations []string `json:"annotations,omitempty"`
	// ExportSettings are the designer's defined export presets, in Figma's
	// own order. `export-assets` defaults to the first one when present,
	// instead of guessing format/scale.
	ExportSettings []ExportSetting `json:"exportSettings,omitempty"`
}

// DevResource is one designer-attached dev-resource link.
type DevResource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ExportSetting is one designer-defined export preset.
type ExportSetting struct {
	Format          string   `json:"format"` // JPG | PNG | SVG
	Suffix          string   `json:"suffix,omitempty"`
	ConstraintType  string   `json:"constraintType,omitempty"` // SCALE | WIDTH | HEIGHT
	ConstraintValue *float64 `json:"constraintValue,omitempty"`
}

// Reaction is one prototyping reaction: what triggers it, and (when the
// action is a NODE navigation with a transition) the transition's
// type/easing/duration.
type Reaction struct {
	Trigger        string   `json:"trigger"`
	TransitionType string   `json:"transitionType,omitempty"`
	Easing         string   `json:"easing,omitempty"`
	Duration       *float64 `json:"duration,omitempty"`
}

// File identifies a Figma file connected to the source.
type File struct {
	FileKey  string `json:"fileKey"`
	FileName string `json:"fileName"`
}

// Page is one top-level page in a Figma file.
type Page struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Metadata is the file-level info returned by the bridge's get_metadata —
// lightweight discovery, no styles, no tree, so an agent can get oriented in
// a file before drilling into any specific node.
type Metadata struct {
	FileName        string `json:"fileName"`
	CurrentPageID   string `json:"currentPageId"`
	CurrentPageName string `json:"currentPageName"`
	Pages           []Page `json:"pages"`
}

// VariableMode is one mode (e.g. "Light"/"Dark") within a VariableCollection.
type VariableMode struct {
	ModeID string `json:"modeId"`
	Name   string `json:"name"`
}

// Variable is one Figma Variable's raw definition: its resolved type and its
// value in each mode of its collection. ValuesByMode entries are passed
// through as-is — a number/string/boolean, a {type:"COLOR",r,g,b,a} object,
// or a {type:"VARIABLE_ALIAS",id} object — no reshaping, ground truth only.
type Variable struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	ResolvedType string         `json:"resolvedType"`
	ValuesByMode map[string]any `json:"valuesByMode"`
	// CodeSyntax holds designer-set code identifiers per platform (WEB,
	// ANDROID, iOS), e.g. {"WEB": "--color-brand-primary"}. When populated,
	// this is the most direct signal for what to call this in code — no
	// guessing needed.
	CodeSyntax map[string]string `json:"codeSyntax,omitempty"`
	// Scopes lists which property types this variable is intended for
	// (e.g. "CORNER_RADIUS", "ALL_FILLS"), as set by the designer in Figma.
	Scopes []string `json:"scopes,omitempty"`
}

// VariableCollection is one collection (a named group of variables sharing a
// set of modes) from the bridge's get_variable_defs.
type VariableCollection struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Modes     []VariableMode `json:"modes"`
	Variables []Variable     `json:"variables"`
}

// VariableDefs is the full local-variable catalog for a file: every
// collection, every variable, every mode value. This is "what tokens exist
// in this file" — independent of any specific node's bindings.
type VariableDefs struct {
	Collections []VariableCollection `json:"collections"`
}

// ScreenshotOpts controls how a node is rendered to an image.
type ScreenshotOpts struct {
	// Format is "PNG" (default), "JPG", or "SVG".
	Format string
	// Scale is the export scale factor; 0 means the backend default.
	Scale float64
}

// Source abstracts access to Figma data. Implementations are responsible for
// transport and for mapping their wire format into the domain Node model. Every
// call takes a context so callers can cancel or time out in-flight requests.
type Source interface {
	// Ping reports whether the source is reachable.
	Ping(ctx context.Context) error
	// Files lists the Figma files currently reachable through the source.
	Files(ctx context.Context) ([]File, error)
	// Document returns the current page's node tree for the given file.
	Document(ctx context.Context, fileKey string) (*Node, error)
	// Node returns a single node by id.
	Node(ctx context.Context, fileKey, id string) (*Node, error)
	// Selection returns the nodes currently selected in the editor.
	Selection(ctx context.Context, fileKey string) ([]Node, error)
	// Screenshot renders a node to image bytes.
	Screenshot(ctx context.Context, fileKey, id string, opts ScreenshotOpts) ([]byte, error)
	// Metadata returns file-level info (pages, current page) — no tree, no
	// styles. The discovery entry point for an agent with no node id yet.
	Metadata(ctx context.Context, fileKey string) (Metadata, error)
	// VariableDefs returns the file's full local-variable catalog (every
	// collection/variable/mode), independent of any specific node.
	VariableDefs(ctx context.Context, fileKey string) (VariableDefs, error)
}

// TopLevelFrames returns the direct FRAME children of the document root. On the
// shadcn Components page these are the per-component sections (e.g. "Button").
func (n *Node) TopLevelFrames() []Node {
	var frames []Node
	for _, c := range n.Children {
		if c.Type == "FRAME" {
			frames = append(frames, c)
		}
	}
	return frames
}

// FirstText walks the subtree depth-first and returns the first non-empty text
// content found, or "". Used to surface a label (e.g. a button's caption) as an
// extra signal for the vision model and for codegen children.
func (n *Node) FirstText() string {
	if n.Type == "TEXT" && n.Characters != "" {
		return n.Characters
	}
	for i := range n.Children {
		if t := n.Children[i].FirstText(); t != "" {
			return t
		}
	}
	return ""
}

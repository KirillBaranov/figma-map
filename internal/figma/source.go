// Package figma defines the domain model for a Figma document and the Source
// interface that abstracts where that data comes from. The bridge backend
// (figma-bridge) is the only implementation today; a REST backend can be
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

// NodeOptions controls what optional, non-default-cheap fields a node fetch
// includes — see NodeWithOptions. Zero value is the cheapest possible fetch
// (matches plain Node/NodeWithDepth exactly).
type NodeOptions struct {
	// Depth caps recursion (0 = unlimited) — same meaning as NodeWithDepth.
	Depth int
	// RenderBounds requests each node's post-effects, post-transform render
	// bounds (Figma's absoluteRenderBounds) alongside the usual declared
	// Bounds. Reading it forces Figma to render the node, so it costs more
	// — opt-in only. Used to geo-diff transform-composition errors (see
	// service.Reconcile's geoDiff option): a rotated/skewed element's
	// declared values can match while the actual rendered box is off,
	// e.g. a wrong transform-origin.
	RenderBounds bool
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
	// RenderBounds is the node's post-effects, post-transform render bounds
	// (Figma's absoluteRenderBounds), in absolute page coordinates — only
	// present when fetched via NodeOptions.RenderBounds. nil, not zero-value,
	// means "not requested/unavailable", so callers can tell "no geo-diff
	// data" apart from "a genuinely zero-sized box".
	RenderBounds *Bounds `json:"renderBounds,omitempty"`
	Styles       *Style  `json:"styles,omitempty"`
	Children   []Node `json:"children,omitempty"`
	// ChildCount is set instead of Children when a depth-limited fetch (see
	// NodeWithDepth) truncates below this node — an honest "N more children
	// exist but weren't walked", not a silent leaf.
	ChildCount int `json:"childCount,omitempty"`
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
	// TextPath is set only on TEXT_PATH nodes ("Text on Path") — the curve
	// the text flows along, as SVG path data. The Plugin API exposes it
	// (TextPathNode.vectorPaths) even though it's absent from every other
	// surface (REST, Dev Mode, static SVG export flattens it into per-glyph
	// outlines instead), so without this the curve looked unrecoverable
	// short of eyeballing it against a screenshot.
	TextPath *TextPath `json:"textPath,omitempty"`
}

// TextPath is the curve a TEXT_PATH node's text flows along.
type TextPath struct {
	VectorPaths       []VectorPath      `json:"vectorPaths"`
	TextPathStartData TextPathStartData `json:"textPathStartData"`
}

// VectorPath is one path segment in Figma's reduced SVG path-data dialect
// (M/L/Q/C/Z only — see Figma's VectorPath docs).
type VectorPath struct {
	WindingRule string `json:"windingRule"`
	Data        string `json:"data"`
}

// TextPathStartData locates where a TEXT_PATH node's text begins along its
// curve: which path segment, and how far (0-1) along that segment.
type TextPathStartData struct {
	Segment  int     `json:"segment"`
	Position float64 `json:"position"`
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
// type/easing/duration/destination.
type Reaction struct {
	Trigger        string   `json:"trigger"`
	TransitionType string   `json:"transitionType,omitempty"`
	Easing         string   `json:"easing,omitempty"`
	Duration       *float64 `json:"duration,omitempty"`
	// DestinationID is the target node of a NODE-navigation action — cheap
	// to carry here since it's already read off the action. Resolving what
	// actually changes at that destination (before/after style diff) is
	// comparatively expensive and lives behind the separate Animation op
	// instead of running for every node a tree walk touches.
	DestinationID string `json:"destinationId,omitempty"`
}

// File identifies a Figma file connected to the source.
type File struct {
	FileKey  string `json:"fileKey"`
	FileName string `json:"fileName"`
	// Status is "connected", "dormant" (a request is going without real
	// progress — usually Figma throttling a backgrounded window, not an
	// error), or "" on a backend that predates this field (REST source
	// never sets it either — REST has no live connection to be dormant).
	Status string `json:"status,omitempty"`
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
	// DocumentWithDepth returns the current page's node tree, recursion
	// capped at depth (0 = unlimited, same as Document). Use this over
	// Document when only a shallow view is needed (e.g. `bind`, which only
	// reads direct FRAME children), so the source doesn't fully serialize
	// every section's subtree just to discard it.
	DocumentWithDepth(ctx context.Context, fileKey string, depth int) (*Node, error)
	// Node returns a single node by id, fully recursed.
	Node(ctx context.Context, fileKey, id string) (*Node, error)
	// NodeWithDepth returns a single node by id, recursion capped at depth
	// (0 = unlimited, same as Node). Nodes beyond the cap report ChildCount
	// instead of Children — use this over Node when the subtree could be
	// large and the caller only needs a shallow view (e.g. `inspect --depth`,
	// `find --within`), so the source doesn't walk/serialize work that would
	// just be discarded.
	NodeWithDepth(ctx context.Context, fileKey, id string, depth int) (*Node, error)
	// NodeWithOptions returns a single node by id with fine-grained control
	// over what gets fetched (see NodeOptions) — Node/NodeWithDepth are
	// convenience wrappers over the common cases and stay in this interface
	// unchanged; reach for this one when a caller needs more than depth
	// (e.g. RenderBounds, for geo-diffing transform-composition errors).
	NodeWithOptions(ctx context.Context, fileKey, id string, opts NodeOptions) (*Node, error)
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
	// FindNodes searches fileKey's tree for nodes matching opts, filtering
	// inside the plugin sandbox — the only way to search a whole-document
	// tree without paying full style/variable serialization for every node
	// (get_document/get_node have no such filter and routinely exceed the
	// bridge's 30s per-request timeout on a non-trivial file).
	FindNodes(ctx context.Context, fileKey string, opts FindNodesOptions) ([]FindMatch, error)
	// MainComponentName resolves an INSTANCE's main-component family name —
	// the COMPONENT_SET name for a variant, or the main component's own name
	// otherwise — empty if id isn't an INSTANCE or its main component can't
	// be resolved. Deliberately a separate call rather than a Node field: it
	// is only ever needed for the one node actually being matched (bind/map/
	// plan's tier-1 name match), never for every INSTANCE in a bulk tree
	// fetch.
	MainComponentName(ctx context.Context, fileKey, id string) (string, error)
	// Animation resolves each of a node's prototyping reactions to an actual
	// before/after style delta (not just trigger/timing, which Node.Reactions
	// already carries cheaply for every node). Deliberately a separate call,
	// like MainComponentName: resolving a destination node (or guessing a
	// state-sibling variant) and diffing full style sets is real async work
	// that should only run for the one node an agent is actually asking about.
	Animation(ctx context.Context, fileKey, id string) ([]Animation, error)
}

// Animation is one prototyping reaction resolved to what actually changes.
type Animation struct {
	Trigger        string   `json:"trigger"`
	TransitionType string   `json:"transitionType,omitempty"`
	Easing         string   `json:"easing,omitempty"`
	Duration       *float64 `json:"duration,omitempty"`
	DestinationID  string   `json:"destinationId,omitempty"`
	// ResolvedVia is "destination" (a real NODE-navigation target — ground
	// truth) or "variant-sibling" (no destination on the reaction itself; a
	// same-component-set sibling was guessed from a state-sounding property
	// name — a best-effort match, not a designer-declared one). Empty when
	// no "after" state could be resolved at all.
	ResolvedVia string               `json:"resolvedVia,omitempty"`
	StyleDelta  *AnimationStyleDelta `json:"styleDelta,omitempty"`
}

// AnimationStyleDelta holds only the style keys that actually differ between
// the reaction's "before" and "after" state, each side keyed by the same
// normalized style field names Tokens/Style already use (opacity, fills,
// cornerRadius, rotation, ...). Left as generic maps rather than a typed
// struct: the set of keys present varies per node/animation, and the
// consumer here is JSON passed straight through to the CLI/MCP caller, not
// further Go-side logic that would benefit from static field access.
type AnimationStyleDelta struct {
	From map[string]any `json:"from,omitempty"`
	To   map[string]any `json:"to,omitempty"`
}

// FindNodesOptions controls a FindNodes search.
type FindNodesOptions struct {
	// Query matches against node name (case-insensitive substring). Empty = match all.
	Query string
	// TextQuery additionally requires the node's text content to contain this string.
	TextQuery string
	// NodeType filters to nodes of this Figma type (e.g. "FRAME", "TEXT", "INSTANCE").
	// Empty means all types.
	NodeType string
	// Mode filters to nodes that have a variable mode override whose name contains this string.
	Mode string
	// WithinNodeID restricts the search to a specific node subtree. Empty means the whole document.
	WithinNodeID string
	// MaxDepth caps recursion depth relative to the search root (0 = unlimited).
	MaxDepth int
	// MaxResults caps how many matches are returned (0 = source default).
	MaxResults int
}

// FindMatch is one node returned by FindNodes — already filtered, and
// deliberately lean: no styles, no bound-variable resolution, no
// dev-resources, since a search doesn't need any of that.
type FindMatch struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	// Path is the breadcrumb from the search root to this node's parent,
	// e.g. "Page › Frame › Group" — ancestors only, not including this node.
	Path           string            `json:"path"`
	Characters     string            `json:"characters,omitempty"`
	VariantModes   map[string]string `json:"variantModes,omitempty"`
	ComponentProps map[string]any    `json:"componentProps,omitempty"`
	DevStatus      string            `json:"devStatus,omitempty"`
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

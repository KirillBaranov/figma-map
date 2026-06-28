package figma

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Style holds the design tokens the bridge serializes for a node: colors,
// spacing, radius, layout, and typography. These are the exact "should-be"
// values used by `tokens` and by reconcile's deterministic Tier 1.
//
// Fields the bridge may emit as the literal "mixed" use MaybeNum so decoding
// never fails on the number|"mixed" union.
type Style struct {
	Opacity *float64 `json:"opacity,omitempty"`
	Visible *bool    `json:"visible,omitempty"`
	// BlendMode mirrors Figma's blend mode (NORMAL, MULTIPLY, SCREEN, ...).
	BlendMode string `json:"blendMode,omitempty"`

	Fills   MaybePaints `json:"fills,omitempty"`
	Strokes MaybePaints `json:"strokes,omitempty"`

	StrokeWeight MaybeNum `json:"strokeWeight,omitempty"`
	StrokeAlign  string   `json:"strokeAlign,omitempty"`
	// StrokeWeights holds per-side stroke widths when they differ; a uniform
	// weight stays in StrokeWeight above.
	StrokeWeights *Sides `json:"strokeWeights,omitempty"`
	// DashPattern is the stroke's dash/gap lengths in px, e.g. [4, 2] for a
	// dashed border. Empty/absent means a solid stroke.
	DashPattern []float64 `json:"dashPattern,omitempty"`

	CornerRadius MaybeNum `json:"cornerRadius,omitempty"`
	CornerRadii  *Corners `json:"cornerRadii,omitempty"`
	// CornerSmoothing is Figma's "squircle" superellipse smoothing (0-1),
	// not representable in CSS border-radius — surfaced for completeness.
	CornerSmoothing *float64 `json:"cornerSmoothing,omitempty"`

	AutoLayout *AutoLayout `json:"autoLayout,omitempty"`
	Padding    *Padding    `json:"padding,omitempty"`

	// ClipsContent mirrors Figma's "Clip content" toggle — false means
	// children may render outside this frame's bounds (no overflow:hidden).
	ClipsContent *bool `json:"clipsContent,omitempty"`
	// Constraints is the node's resize-behavior pins relative to its parent
	// (e.g. RIGHT/BOTTOM means it tracks the opposite edge, not left/top).
	Constraints *Constraints `json:"constraints,omitempty"`

	// Auto-layout child escape hatches (only set for non-default values).
	// LayoutPositioning="ABSOLUTE" means this child ignores the parent's
	// flex flow entirely, even though the parent itself is auto-layout.
	LayoutPositioning string   `json:"layoutPositioning,omitempty"`
	LayoutGrow        *float64 `json:"layoutGrow,omitempty"`
	LayoutAlign       string   `json:"layoutAlign,omitempty"`

	Effects  []Effect `json:"effects,omitempty"`
	Rotation *float64 `json:"rotation,omitempty"`

	// Typography (present on TEXT nodes).
	FontSize       MaybeNum `json:"fontSize,omitempty"`
	FontFamily     string   `json:"fontFamily,omitempty"`
	FontStyle      string   `json:"fontStyle,omitempty"`
	FontWeight     MaybeNum `json:"fontWeight,omitempty"`
	TextDecoration string   `json:"textDecoration,omitempty"`
	// TextCase mirrors Figma's text-case style: ORIGINAL | UPPER | LOWER | TITLE |
	// SMALL_CAPS | SMALL_CAPS_FORCED. It is applied via CSS text-transform rather
	// than baked into Characters, so Figma's displayed text and the raw
	// characters can differ (e.g. "Преимущества" displayed as "ПРЕИМУЩЕСТВА").
	TextCase            string `json:"textCase,omitempty"`
	LineHeight          *Unit  `json:"lineHeight,omitempty"`
	LetterSpacing       *Unit  `json:"letterSpacing,omitempty"`
	TextAlignHorizontal string `json:"textAlignHorizontal,omitempty"`
	TextAlignVertical   string `json:"textAlignVertical,omitempty"`

	// BoundVariables maps a Figma field name (e.g. "itemSpacing",
	// "topLeftRadius", "paddingLeft", "strokeWeight", "opacity" — Figma's own
	// names, not a CSS property) to the "Collection/Name" of the Variable
	// directly bound to it. fills/strokes are excluded here — see each
	// Paint's own Variable field, which is per-paint and more precise.
	BoundVariables map[string]string `json:"boundVariables,omitempty"`
}

// Paint is a single fill or stroke. SOLID paints carry a hex Color; other
// paint types (gradients, images) are kept loosely and surfaced as their Type.
type Paint struct {
	Type    string  `json:"type"`
	Color   string  `json:"color,omitempty"` // hex, e.g. "#18181b" (SOLID)
	Opacity float64 `json:"opacity,omitempty"`
	// Variable is the "Collection/Name" of the Figma Variable this paint's
	// color is directly bound to, if any — e.g. "Color/Brand/Primary" for a
	// fill that's #18181b only because that variable currently resolves to
	// it. Empty means this is a literal, unbound color.
	Variable string `json:"variable,omitempty"`
	// CodeSyntax is that variable's designer-set WEB code identifier (e.g.
	// "--color-brand-primary"), if any. When set, CSSColor() emits
	// var(--color-brand-primary) instead of the literal hex — ground truth
	// for the CSS variable's name, not a guess.
	CodeSyntax string `json:"codeSyntax,omitempty"`
}

// CSSColor returns the paint's color as a CSS value: var(--token) when this
// paint is bound to a Variable with a designer-set WEB CodeSyntax (ground
// truth for the project's own token name — preferred whenever the paint is
// otherwise opaque enough to substitute cleanly), the plain hex for any
// other opaque paint, or rgba(...) when the paint itself carries fractional
// opacity (e.g. a 10% tint with backdrop blur) — that opacity is on the
// paint, separate from the node's own Style.Opacity, and is otherwise lost
// when only Color is read. A var() reference can't absorb that extracted
// alpha, so CodeSyntax is only used in the simple opaque case.
func (p Paint) CSSColor() string {
	if p.Type != "SOLID" || p.Color == "" {
		return ""
	}
	if p.Opacity <= 0 || p.Opacity >= 1 {
		if p.CodeSyntax != "" {
			return fmt.Sprintf("var(%s)", cssCustomPropertyName(p.CodeSyntax))
		}
		return p.Color
	}
	r, g, b, ok := hexToRGB(p.Color)
	if !ok {
		return p.Color
	}
	// Figma stores opacity as float32; rounding to 3 decimals kills the
	// float32→float64 conversion noise (e.g. 0.10000000149011612 → 0.1).
	alpha := math.Round(p.Opacity*1000) / 1000
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", r, g, b, strconv.FormatFloat(alpha, 'g', -1, 64))
}

// FirstSolidCSS returns the CSS color of the first SOLID paint in paints, or "".
func FirstSolidCSS(paints []Paint) string {
	for _, p := range paints {
		if c := p.CSSColor(); c != "" {
			return c
		}
	}
	return ""
}

// FirstSolidVariable returns the bound-variable label ("Collection/Name") of
// the same paint FirstSolidCSS would report a color for, or "" if that paint
// isn't bound to a variable (a literal color) or there is no solid paint.
func FirstSolidVariable(paints []Paint) string {
	for _, p := range paints {
		if p.CSSColor() != "" {
			return p.Variable
		}
	}
	return ""
}

// cssCustomPropertyName ensures a Variable's CodeSyntax reads as a valid CSS
// custom property name for var(...) — designers sometimes set CodeSyntax
// without the leading "--" (e.g. "color-brand-primary" rather than
// "--color-brand-primary"); var() requires it.
func cssCustomPropertyName(codeSyntax string) string {
	if strings.HasPrefix(codeSyntax, "--") {
		return codeSyntax
	}
	return "--" + codeSyntax
}

func hexToRGB(hex string) (r, g, b int, ok bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff), true
}

// Corners holds per-corner radii when they differ.
type Corners struct {
	TopLeft     float64 `json:"topLeft"`
	TopRight    float64 `json:"topRight"`
	BottomRight float64 `json:"bottomRight"`
	BottomLeft  float64 `json:"bottomLeft"`
}

// AutoLayout mirrors Figma auto-layout, the basis for flex/grid codegen.
type AutoLayout struct {
	Direction        string  `json:"direction"` // HORIZONTAL | VERTICAL | GRID
	Gap              float64 `json:"gap"`
	PrimaryAxisAlign string  `json:"primaryAxisAlign,omitempty"`
	CounterAxisAlign string  `json:"counterAxisAlign,omitempty"`
	Wrap             string  `json:"wrap,omitempty"`
	CounterAxisGap   float64 `json:"counterAxisSpacing,omitempty"`
	// PrimaryAxisSizing/CounterAxisSizing are FIXED or AUTO. FIXED means Figma
	// carries an explicit pixel size on that axis (vs. AUTO, which hugs
	// content) — needed so wrapped text breaks at the same width as the design.
	PrimaryAxisSizing string `json:"primaryAxisSizing,omitempty"`
	CounterAxisSizing string `json:"counterAxisSizing,omitempty"`

	// Set only when Direction is "GRID" — Figma's native CSS-grid-like
	// auto-layout. This is ground truth (the designer's own explicit
	// row/column setup), never inferred from freeform positioning.
	GridRowSizes    []GridTrack `json:"gridRowSizes,omitempty"`
	GridColumnSizes []GridTrack `json:"gridColumnSizes,omitempty"`
	GridRowGap      float64     `json:"gridRowGap,omitempty"`
	GridColumnGap   float64     `json:"gridColumnGap,omitempty"`
}

// GridTrack is one row or column of a GRID auto-layout frame, mirroring
// Figma's GridTrackSize: FIXED carries a px Value, FLEX an fr-equivalent
// Value, HUG sizes to content (Value unused).
type GridTrack struct {
	Type  string   `json:"type"` // FLEX | FIXED | HUG
	Value *float64 `json:"value,omitempty"`
}

// GridPosition is a node's explicit row/column placement within its
// parent's GRID auto-layout — only present on direct children of one.
type GridPosition struct {
	RowIndex    int `json:"rowIndex"`
	ColumnIndex int `json:"columnIndex"`
	RowSpan     int `json:"rowSpan"`
	ColumnSpan  int `json:"columnSpan"`
}

// Constraints mirrors Figma's per-axis resize constraints: MIN | MAX | CENTER
// | STRETCH | SCALE. MIN/MAX pin to the near/far edge of the parent (e.g. a
// horizontal MAX constraint should become `right` instead of `left` for an
// absolutely-positioned child); STRETCH/SCALE track the parent's resize.
type Constraints struct {
	Horizontal string `json:"horizontal"`
	Vertical   string `json:"vertical"`
}

// Padding is the auto-layout padding box.
type Padding struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

// Sides is a generic per-side value, e.g. per-side stroke weights.
type Sides struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

// Effect is a drop/inner shadow or blur.
type Effect struct {
	Type   string  `json:"type"`
	Radius float64 `json:"radius,omitempty"`
	Color  string  `json:"color,omitempty"`
}

// Unit is a {unit,value} pair (e.g. lineHeight "PIXELS"/24, or "AUTO").
type Unit struct {
	Unit  string  `json:"unit"`
	Value float64 `json:"value,omitempty"`
}

// MaybePaints decodes a JSON value that is either []Paint or the string "mixed".
// When Mixed=true the paints slice is nil; callers should treat it as unknown.
type MaybePaints struct {
	Paints []Paint
	Mixed  bool
}

// UnmarshalJSON implements the []Paint|"mixed" union.
func (m *MaybePaints) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if string(b) == `"mixed"` {
		m.Mixed = true
		return nil
	}
	return json.Unmarshal(b, &m.Paints)
}

// MaybeNum decodes a JSON value that is either a number or the string "mixed".
// Set reports whether a value was present; Mixed reports the "mixed" sentinel.
type MaybeNum struct {
	Value float64
	Mixed bool
	Set   bool
}

// UnmarshalJSON implements the number|"mixed"|null union.
func (m *MaybeNum) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch string(b) {
	case "null", "":
		return nil
	case `"mixed"`:
		m.Mixed, m.Set = true, true
		return nil
	}
	if err := json.Unmarshal(b, &m.Value); err != nil {
		return err
	}
	m.Set = true
	return nil
}

// MarshalJSON emits the number, "mixed", or null.
func (m MaybeNum) MarshalJSON() ([]byte, error) {
	switch {
	case !m.Set:
		return []byte("null"), nil
	case m.Mixed:
		return []byte(`"mixed"`), nil
	default:
		return json.Marshal(m.Value)
	}
}

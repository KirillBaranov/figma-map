package figma

import (
	"bytes"
	"encoding/json"
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

	Fills   []Paint `json:"fills,omitempty"`
	Strokes []Paint `json:"strokes,omitempty"`

	StrokeWeight MaybeNum `json:"strokeWeight,omitempty"`
	StrokeAlign  string   `json:"strokeAlign,omitempty"`

	CornerRadius MaybeNum `json:"cornerRadius,omitempty"`
	CornerRadii  *Corners `json:"cornerRadii,omitempty"`

	AutoLayout *AutoLayout `json:"autoLayout,omitempty"`
	Padding    *Padding    `json:"padding,omitempty"`

	Effects  []Effect `json:"effects,omitempty"`
	Rotation *float64 `json:"rotation,omitempty"`

	// Typography (present on TEXT nodes).
	FontSize            MaybeNum `json:"fontSize,omitempty"`
	FontFamily          string   `json:"fontFamily,omitempty"`
	FontStyle           string   `json:"fontStyle,omitempty"`
	FontWeight          MaybeNum `json:"fontWeight,omitempty"`
	TextDecoration      string   `json:"textDecoration,omitempty"`
	LineHeight          *Unit    `json:"lineHeight,omitempty"`
	LetterSpacing       *Unit    `json:"letterSpacing,omitempty"`
	TextAlignHorizontal string   `json:"textAlignHorizontal,omitempty"`
	TextAlignVertical   string   `json:"textAlignVertical,omitempty"`
}

// Paint is a single fill or stroke. SOLID paints carry a hex Color; other
// paint types (gradients, images) are kept loosely and surfaced as their Type.
type Paint struct {
	Type    string  `json:"type"`
	Color   string  `json:"color,omitempty"` // hex, e.g. "#18181b" (SOLID)
	Opacity float64 `json:"opacity,omitempty"`
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
	Direction        string  `json:"direction"` // HORIZONTAL | VERTICAL
	Gap              float64 `json:"gap"`
	PrimaryAxisAlign string  `json:"primaryAxisAlign,omitempty"`
	CounterAxisAlign string  `json:"counterAxisAlign,omitempty"`
	Wrap             string  `json:"wrap,omitempty"`
	CounterAxisGap   float64 `json:"counterAxisSpacing,omitempty"`
}

// Padding is the auto-layout padding box.
type Padding struct {
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

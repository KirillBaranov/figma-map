package figma

import "fmt"

// restNode mirrors the subset of Figma REST API's node JSON shape
// RESTSource maps into the domain Node model (see rest_source.go's doc
// comment for what's deliberately left out: bound-variable resolution
// beyond fills/strokes, prototyping Reactions, DevResources, Annotations,
// GridPosition).
type restNode struct {
	ID                    string                      `json:"id"`
	Name                  string                      `json:"name"`
	Type                  string                      `json:"type"`
	Characters            string                      `json:"characters,omitempty"`
	AbsoluteBoundingBox   *restRect                   `json:"absoluteBoundingBox,omitempty"`
	Children              []restNode                  `json:"children,omitempty"`
	Visible               *bool                       `json:"visible,omitempty"`
	Opacity               *float64                    `json:"opacity,omitempty"`
	BlendMode             string                      `json:"blendMode,omitempty"`
	Fills                 []restPaint                 `json:"fills,omitempty"`
	Strokes               []restPaint                 `json:"strokes,omitempty"`
	StrokeWeight          *float64                    `json:"strokeWeight,omitempty"`
	StrokeAlign           string                      `json:"strokeAlign,omitempty"`
	CornerRadius          *float64                    `json:"cornerRadius,omitempty"`
	RectangleCornerRadii  []float64                   `json:"rectangleCornerRadii,omitempty"`
	LayoutMode            string                      `json:"layoutMode,omitempty"`
	ItemSpacing           *float64                    `json:"itemSpacing,omitempty"`
	CounterAxisSpacing    *float64                    `json:"counterAxisSpacing,omitempty"`
	PaddingLeft           *float64                    `json:"paddingLeft,omitempty"`
	PaddingRight          *float64                    `json:"paddingRight,omitempty"`
	PaddingTop            *float64                    `json:"paddingTop,omitempty"`
	PaddingBottom         *float64                    `json:"paddingBottom,omitempty"`
	PrimaryAxisAlignItems string                      `json:"primaryAxisAlignItems,omitempty"`
	CounterAxisAlignItems string                      `json:"counterAxisAlignItems,omitempty"`
	LayoutWrap            string                      `json:"layoutWrap,omitempty"`
	Style                 *restTypeStyle              `json:"style,omitempty"`
	Effects               []restEffect                `json:"effects,omitempty"`
	ComponentID           string                      `json:"componentId,omitempty"`
	ComponentProperties   map[string]restCompProperty `json:"componentProperties,omitempty"`
	ExportSettings        []restExportSetting         `json:"exportSettings,omitempty"`
}

type restRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type restColor struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}

type restPaint struct {
	Type    string     `json:"type"`
	Color   *restColor `json:"color,omitempty"`
	Opacity *float64   `json:"opacity,omitempty"`
}

type restTypeStyle struct {
	FontFamily          string  `json:"fontFamily,omitempty"`
	FontWeight          float64 `json:"fontWeight,omitempty"`
	FontSize            float64 `json:"fontSize,omitempty"`
	TextAlignHorizontal string  `json:"textAlignHorizontal,omitempty"`
	TextAlignVertical   string  `json:"textAlignVertical,omitempty"`
	LetterSpacing       float64 `json:"letterSpacing,omitempty"`
	LineHeightPx        float64 `json:"lineHeightPx,omitempty"`
}

type restEffect struct {
	Type    string     `json:"type"`
	Radius  float64    `json:"radius,omitempty"`
	Color   *restColor `json:"color,omitempty"`
	Visible *bool      `json:"visible,omitempty"`
}

type restCompProperty struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type restExportSetting struct {
	Suffix     string `json:"suffix,omitempty"`
	Format     string `json:"format"`
	Constraint struct {
		Type  string  `json:"type"`
		Value float64 `json:"value"`
	} `json:"constraint"`
}

// toDomain maps a REST node (and, depth-limited, its children) into the
// source-independent Node model. Fields with no REST equivalent, or not yet
// mapped (see rest_source.go's doc comment), are left at their zero value —
// omitempty means they simply don't appear, never a fabricated value.
func (n restNode) toDomain() Node {
	node := Node{
		ID:         n.ID,
		Name:       n.Name,
		Type:       n.Type,
		Characters: n.Characters,
	}
	if n.AbsoluteBoundingBox != nil {
		node.Bounds = Bounds{
			X:      n.AbsoluteBoundingBox.X,
			Y:      n.AbsoluteBoundingBox.Y,
			Width:  n.AbsoluteBoundingBox.Width,
			Height: n.AbsoluteBoundingBox.Height,
		}
	}
	if len(n.Children) > 0 {
		node.Children = make([]Node, len(n.Children))
		for i, c := range n.Children {
			node.Children[i] = c.toDomain()
		}
	}
	if len(n.ComponentProperties) > 0 {
		node.ComponentProps = make(map[string]any, len(n.ComponentProperties))
		for k, p := range n.ComponentProperties {
			node.ComponentProps[k] = p.Value
		}
	}
	if len(n.ExportSettings) > 0 {
		node.ExportSettings = make([]ExportSetting, len(n.ExportSettings))
		for i, e := range n.ExportSettings {
			var constraintValue *float64
			if e.Constraint.Value != 0 {
				v := e.Constraint.Value
				constraintValue = &v
			}
			node.ExportSettings[i] = ExportSetting{
				Format:          e.Format,
				Suffix:          e.Suffix,
				ConstraintType:  e.Constraint.Type,
				ConstraintValue: constraintValue,
			}
		}
	}
	node.Styles = n.toStyle()
	return node
}

// toStyle maps the subset of a REST node's style-bearing fields Style
// covers. Returns nil when the node carries none of them (e.g. a bare
// GROUP), matching the bridge's own "no Styles" convention for such nodes.
func (n restNode) toStyle() *Style {
	s := &Style{
		Opacity:     n.Opacity,
		Visible:     n.Visible,
		BlendMode:   n.BlendMode,
		StrokeAlign: n.StrokeAlign,
	}
	hasStyle := s.Opacity != nil || s.Visible != nil || s.BlendMode != ""

	if len(n.Fills) > 0 {
		s.Fills = MaybePaints{Paints: mapPaints(n.Fills)}
		hasStyle = true
	}
	if len(n.Strokes) > 0 {
		s.Strokes = MaybePaints{Paints: mapPaints(n.Strokes)}
		hasStyle = true
	}
	if n.StrokeWeight != nil {
		s.StrokeWeight = MaybeNum{Value: *n.StrokeWeight, Set: true}
		hasStyle = true
	}
	if n.CornerRadius != nil {
		s.CornerRadius = MaybeNum{Value: *n.CornerRadius, Set: true}
		hasStyle = true
	}
	if len(n.RectangleCornerRadii) == 4 {
		s.CornerRadii = &Corners{
			TopLeft:     n.RectangleCornerRadii[0],
			TopRight:    n.RectangleCornerRadii[1],
			BottomRight: n.RectangleCornerRadii[2],
			BottomLeft:  n.RectangleCornerRadii[3],
		}
		hasStyle = true
	}
	if n.LayoutMode != "" && n.LayoutMode != "NONE" {
		s.AutoLayout = &AutoLayout{
			Direction:        n.LayoutMode,
			PrimaryAxisAlign: n.PrimaryAxisAlignItems,
			CounterAxisAlign: n.CounterAxisAlignItems,
			Wrap:             n.LayoutWrap,
		}
		if n.ItemSpacing != nil {
			s.AutoLayout.Gap = *n.ItemSpacing
		}
		if n.CounterAxisSpacing != nil {
			s.AutoLayout.CounterAxisGap = *n.CounterAxisSpacing
		}
		hasStyle = true
	}
	if n.PaddingLeft != nil || n.PaddingRight != nil || n.PaddingTop != nil || n.PaddingBottom != nil {
		s.Padding = &Padding{
			Left:   derefOr(n.PaddingLeft, 0),
			Right:  derefOr(n.PaddingRight, 0),
			Top:    derefOr(n.PaddingTop, 0),
			Bottom: derefOr(n.PaddingBottom, 0),
		}
		hasStyle = true
	}
	if len(n.Effects) > 0 {
		s.Effects = mapEffects(n.Effects)
		hasStyle = true
	}
	if n.Style != nil {
		s.FontFamily = n.Style.FontFamily
		s.FontStyle = ""
		if n.Style.FontSize != 0 {
			s.FontSize = MaybeNum{Value: n.Style.FontSize, Set: true}
		}
		if n.Style.FontWeight != 0 {
			s.FontWeight = MaybeNum{Value: n.Style.FontWeight, Set: true}
		}
		s.TextAlignHorizontal = n.Style.TextAlignHorizontal
		s.TextAlignVertical = n.Style.TextAlignVertical
		if n.Style.LetterSpacing != 0 {
			s.LetterSpacing = &Unit{Unit: "PIXELS", Value: n.Style.LetterSpacing}
		}
		if n.Style.LineHeightPx != 0 {
			s.LineHeight = &Unit{Unit: "PIXELS", Value: n.Style.LineHeightPx}
		}
		hasStyle = true
	}

	if !hasStyle {
		return nil
	}
	return s
}

func mapPaints(paints []restPaint) []Paint {
	out := make([]Paint, len(paints))
	for i, p := range paints {
		mapped := Paint{Type: p.Type}
		if p.Opacity != nil {
			mapped.Opacity = *p.Opacity
		}
		if p.Type == "SOLID" && p.Color != nil {
			mapped.Color = hexFromRESTColor(*p.Color)
		}
		out[i] = mapped
	}
	return out
}

func mapEffects(effects []restEffect) []Effect {
	out := make([]Effect, 0, len(effects))
	for _, e := range effects {
		if e.Visible != nil && !*e.Visible {
			continue
		}
		mapped := Effect{Type: e.Type, Radius: e.Radius}
		if e.Color != nil {
			mapped.Color = hexFromRESTColor(*e.Color)
		}
		out = append(out, mapped)
	}
	return out
}

// hexFromRESTColor converts Figma REST's {r,g,b,a} (each 0..1) to a "#rrggbb"
// hex string — the alpha channel is intentionally dropped here, same as the
// bridge's own Paint.Color (see Paint.Opacity for fractional alpha).
func hexFromRESTColor(c restColor) string {
	return fmt.Sprintf("#%02x%02x%02x", clamp255(c.R), clamp255(c.G), clamp255(c.B))
}

func clamp255(v float64) int {
	n := int(v*255 + 0.5)
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return n
}

func derefOr(v *float64, fallback float64) float64 {
	if v == nil {
		return fallback
	}
	return *v
}

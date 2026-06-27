package service

import (
	"context"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// Tokens is the normalized, code-friendly view of a node's design tokens. All
// values are exact (from the Figma tree), so they are the deterministic
// "should-be" used by reconcile and for hand-building unmapped elements.
type Tokens struct {
	Fill   string `json:"fill,omitempty"`   // hex, first solid fill
	Stroke string `json:"stroke,omitempty"` // hex, first solid stroke
	// FillVariable/StrokeVariable are the "Collection/Name" of the Figma
	// Variable the first solid fill/stroke is directly bound to, if any —
	// empty means Fill/Stroke above is a literal, unbound color.
	FillVariable   string `json:"fillVariable,omitempty"`
	StrokeVariable string `json:"strokeVariable,omitempty"`
	// Variables maps any other directly-bound Figma field (Figma's own
	// names, e.g. "itemSpacing", "topLeftRadius", "opacity" — not remapped to
	// a CSS property, that's still the agent's call) to the bound Variable's
	// "Collection/Name". See `variables` for the file's full token catalog.
	Variables    map[string]string `json:"variables,omitempty"`
	StrokeWeight *float64          `json:"strokeWeight,omitempty"`
	Radius       *float64          `json:"radius,omitempty"`
	Opacity      *float64          `json:"opacity,omitempty"`
	Padding      *figma.Padding    `json:"padding,omitempty"`
	Gap          *float64          `json:"gap,omitempty"`
	Direction    string            `json:"direction,omitempty"`    // row | column (from auto-layout)
	Shadow       bool              `json:"shadow,omitempty"`       // has a drop shadow
	Blur         *float64          `json:"blur,omitempty"`         // layer blur radius (px)
	BackdropBlur *float64          `json:"backdropBlur,omitempty"` // background blur radius (px)

	// Typography (TEXT nodes).
	FontSize      *float64 `json:"fontSize,omitempty"`
	FontFamily    string   `json:"fontFamily,omitempty"`
	FontWeight    *float64 `json:"fontWeight,omitempty"`
	LineHeight    *float64 `json:"lineHeight,omitempty"`
	LetterSpacing *float64 `json:"letterSpacing,omitempty"`
	TextAlign     string   `json:"textAlign,omitempty"`
	// TextTransform is the CSS text-transform value derived from Figma's
	// textCase (uppercase/lowercase/capitalize) — see figma.Style.TextCase.
	TextTransform string `json:"textTransform,omitempty"`
}

// TokensResult is the `tokens` operation output.
type TokensResult struct {
	NodeID string  `json:"nodeId"`
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Tokens *Tokens `json:"tokens,omitempty"`
	// Reactions are this node's prototyping reactions, if any — opt-in
	// detail, never folded into codegen (see figma.Reaction).
	Reactions []figma.Reaction `json:"reactions,omitempty"`
}

// GetTokens returns the normalized tokens for a single node. Deterministic.
func (s *Service) GetTokens(ctx context.Context, fileKey, nodeID string) (TokensResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return TokensResult{}, err
	}
	node, err := s.src.Node(ctx, key, nodeID)
	if err != nil {
		return TokensResult{}, err
	}
	return TokensResult{
		NodeID: node.ID, Name: node.Name, Type: node.Type,
		Tokens:    tokensFromStyle(node.Styles),
		Reactions: node.Reactions,
	}, nil
}

// tokensFromStyle normalizes a raw figma.Style into Tokens, or nil if there is
// nothing useful to report.
func tokensFromStyle(st *figma.Style) *Tokens {
	if st == nil {
		return nil
	}
	t := &Tokens{
		Opacity:    st.Opacity,
		Fill:       figma.FirstSolidCSS(st.Fills.Paints),
		Stroke:     figma.FirstSolidCSS(st.Strokes.Paints),
		FontFamily: st.FontFamily,
		TextAlign:  st.TextAlignHorizontal,
		Variables:  st.BoundVariables,
	}
	if v := figma.FirstSolidVariable(st.Fills.Paints); v != "" {
		t.FillVariable = v
	}
	if v := figma.FirstSolidVariable(st.Strokes.Paints); v != "" {
		t.StrokeVariable = v
	}
	if st.StrokeWeight.Set && !st.StrokeWeight.Mixed {
		t.StrokeWeight = ptr(st.StrokeWeight.Value)
	}
	if st.CornerRadius.Set && !st.CornerRadius.Mixed {
		t.Radius = ptr(st.CornerRadius.Value)
	}
	if st.FontSize.Set && !st.FontSize.Mixed {
		t.FontSize = ptr(st.FontSize.Value)
	}
	if st.FontWeight.Set && !st.FontWeight.Mixed {
		t.FontWeight = ptr(st.FontWeight.Value)
	}
	if st.LineHeight != nil && st.LineHeight.Unit == "PIXELS" {
		t.LineHeight = ptr(st.LineHeight.Value)
	}
	if st.LetterSpacing != nil && st.LetterSpacing.Unit == "PIXELS" {
		t.LetterSpacing = ptr(st.LetterSpacing.Value)
	}
	switch st.TextCase {
	case "UPPER":
		t.TextTransform = "uppercase"
	case "LOWER":
		t.TextTransform = "lowercase"
	case "TITLE":
		t.TextTransform = "capitalize"
	}
	if st.AutoLayout != nil {
		t.Gap = ptr(st.AutoLayout.Gap)
		switch st.AutoLayout.Direction {
		case "HORIZONTAL":
			t.Direction = "row"
		case "VERTICAL":
			t.Direction = "column"
		}
	}
	if st.Padding != nil {
		t.Padding = st.Padding
	}
	for _, e := range st.Effects {
		switch e.Type {
		case "DROP_SHADOW":
			t.Shadow = true
		case "LAYER_BLUR":
			t.Blur = ptr(e.Radius)
		case "BACKGROUND_BLUR":
			t.BackdropBlur = ptr(e.Radius)
		}
	}

	if t.isEmpty() {
		return nil
	}
	return t
}

func (t *Tokens) isEmpty() bool {
	return t.Fill == "" && t.Stroke == "" && t.StrokeWeight == nil && t.Radius == nil &&
		t.Opacity == nil && t.Padding == nil && t.Gap == nil && t.Direction == "" &&
		!t.Shadow && t.Blur == nil && t.BackdropBlur == nil &&
		t.FontSize == nil && t.FontFamily == "" && t.FontWeight == nil &&
		t.LineHeight == nil && t.LetterSpacing == nil && t.TextAlign == "" &&
		t.TextTransform == "" && t.FillVariable == "" && t.StrokeVariable == "" &&
		len(t.Variables) == 0
}

func ptr[T any](v T) *T { return &v }

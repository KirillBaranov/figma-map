package service

import (
	"context"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// Tokens is the normalized, code-friendly view of a node's design tokens. All
// values are exact (from the Figma tree), so they are the deterministic
// "should-be" used by reconcile and for hand-building unmapped elements.
type Tokens struct {
	Fill         string         `json:"fill,omitempty"`   // hex, first solid fill
	Stroke       string         `json:"stroke,omitempty"` // hex, first solid stroke
	StrokeWeight *float64       `json:"strokeWeight,omitempty"`
	Radius       *float64       `json:"radius,omitempty"`
	Opacity      *float64       `json:"opacity,omitempty"`
	Padding      *figma.Padding `json:"padding,omitempty"`
	Gap          *float64       `json:"gap,omitempty"`
	Direction    string         `json:"direction,omitempty"` // row | column (from auto-layout)
	Shadow       bool           `json:"shadow,omitempty"`    // has a drop shadow

	// Typography (TEXT nodes).
	FontSize      *float64 `json:"fontSize,omitempty"`
	FontFamily    string   `json:"fontFamily,omitempty"`
	FontWeight    *float64 `json:"fontWeight,omitempty"`
	LineHeight    *float64 `json:"lineHeight,omitempty"`
	LetterSpacing *float64 `json:"letterSpacing,omitempty"`
	TextAlign     string   `json:"textAlign,omitempty"`
}

// TokensResult is the `tokens` operation output.
type TokensResult struct {
	NodeID string  `json:"nodeId"`
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Tokens *Tokens `json:"tokens,omitempty"`
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
		Tokens: tokensFromStyle(node.Styles),
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
		Fill:       firstSolid(st.Fills),
		Stroke:     firstSolid(st.Strokes),
		FontFamily: st.FontFamily,
		TextAlign:  st.TextAlignHorizontal,
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
		if e.Type == "DROP_SHADOW" {
			t.Shadow = true
			break
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
		!t.Shadow && t.FontSize == nil && t.FontFamily == "" && t.FontWeight == nil &&
		t.LineHeight == nil && t.LetterSpacing == nil && t.TextAlign == ""
}

// firstSolid returns the hex color of the first SOLID paint, or "".
func firstSolid(paints []figma.Paint) string {
	for _, p := range paints {
		if p.Type == "SOLID" && p.Color != "" {
			return p.Color
		}
	}
	return ""
}

func ptr[T any](v T) *T { return &v }

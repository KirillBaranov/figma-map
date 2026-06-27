package service

import (
	"context"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/figma"
)

// TestTokensFromStyle_Variables covers Phase 2: a bound fill surfaces
// FillVariable, and other directly-bound fields pass through as Variables —
// this is the ground truth an agent needs instead of guessing which
// Variable a literal value came from.
func TestTokensFromStyle_Variables(t *testing.T) {
	st := &figma.Style{
		Fills: figma.MaybePaints{Paints: []figma.Paint{
			{Type: "SOLID", Color: "#18181b", Variable: "Color/Brand/Primary"},
		}},
		BoundVariables: map[string]string{"itemSpacing": "Spacing/sm"},
	}

	tok := tokensFromStyle(st)
	if tok == nil || tok.Fill != "#18181b" {
		t.Fatalf("fill = %+v, want #18181b", tok)
	}
	if tok.FillVariable != "Color/Brand/Primary" {
		t.Errorf("fillVariable = %q", tok.FillVariable)
	}
	if tok.Variables["itemSpacing"] != "Spacing/sm" {
		t.Errorf("variables = %+v", tok.Variables)
	}

	// A literal, unbound fill must not fabricate a variable label.
	litTok := tokensFromStyle(&figma.Style{
		Fills: figma.MaybePaints{Paints: []figma.Paint{{Type: "SOLID", Color: "#ffffff"}}},
	})
	if litTok == nil || litTok.FillVariable != "" {
		t.Errorf("literal fill should have empty FillVariable, got %+v", litTok)
	}
}

// TestGetTokens_Reactions covers Phase 4: `tokens` surfaces a node's
// prototyping reactions alongside its style tokens — opt-in detail, not
// folded into codegen.
func TestGetTokens_Reactions(t *testing.T) {
	dur := 0.2
	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		nodes: map[string]*figma.Node{
			"1:1": {
				ID: "1:1", Name: "Button", Type: "INSTANCE",
				Reactions: []figma.Reaction{{Trigger: "ON_HOVER", TransitionType: "SMART_ANIMATE", Duration: &dur}},
			},
		},
	}
	s := &Service{cfg: config.Config{}, src: fake}

	res, err := s.GetTokens(context.Background(), "k", "1:1")
	if err != nil {
		t.Fatalf("GetTokens: %v", err)
	}
	if len(res.Reactions) != 1 || res.Reactions[0].Trigger != "ON_HOVER" {
		t.Errorf("Reactions = %+v", res.Reactions)
	}
}

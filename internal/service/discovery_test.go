package service

import (
	"context"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/figma"
)

func TestPages(t *testing.T) {
	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		metadata: figma.Metadata{
			FileName:        "Design System",
			CurrentPageID:   "0:1",
			CurrentPageName: "Page 1",
			Pages:           []figma.Page{{ID: "0:1", Name: "Page 1"}, {ID: "0:2", Name: "Components"}},
		},
	}
	s := &Service{cfg: config.Config{}, src: fake}

	res, err := s.Pages(context.Background(), "k")
	if err != nil {
		t.Fatalf("Pages: %v", err)
	}
	if res.FileName != "Design System" || len(res.Pages) != 2 {
		t.Errorf("Pages = %+v", res)
	}
}

func TestVariables(t *testing.T) {
	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		variableDefs: figma.VariableDefs{Collections: []figma.VariableCollection{{
			ID: "c1", Name: "Color",
			Modes: []figma.VariableMode{{ModeID: "m1", Name: "Light"}},
			Variables: []figma.Variable{{
				ID: "v1", Name: "Brand/Primary", ResolvedType: "COLOR",
				ValuesByMode: map[string]any{"m1": map[string]any{"type": "COLOR", "r": 0.1, "g": 0.2, "b": 0.3, "a": 1.0}},
				CodeSyntax:   map[string]string{"WEB": "--color-brand-primary"},
				Scopes:       []string{"ALL_FILLS"},
			}},
		}}},
	}
	s := &Service{cfg: config.Config{}, src: fake}

	res, err := s.Variables(context.Background(), "k")
	if err != nil {
		t.Fatalf("Variables: %v", err)
	}
	if len(res.Collections) != 1 || res.Collections[0].Variables[0].Name != "Brand/Primary" {
		t.Errorf("Variables = %+v", res)
	}
	v := res.Collections[0].Variables[0]
	if v.CodeSyntax["WEB"] != "--color-brand-primary" || len(v.Scopes) != 1 || v.Scopes[0] != "ALL_FILLS" {
		t.Errorf("codeSyntax/scopes = %+v", v)
	}
}

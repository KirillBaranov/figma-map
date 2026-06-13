package storybook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIndex(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "index.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	stories, err := ParseIndex(raw)
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	if len(stories) == 0 {
		t.Fatal("expected UI stories, got none")
	}

	// Every parsed story must be under the UI/ prefix and have component+variant.
	var foundButtonPrimary bool
	for _, s := range stories {
		if s.Component == "" || s.Variant == "" {
			t.Errorf("story %s missing component/variant: %+v", s.ID, s)
		}
		if s.ID == "ui-button--primary" {
			foundButtonPrimary = true
			if s.Component != "Button" {
				t.Errorf("expected component Button, got %q", s.Component)
			}
			if s.Variant != "Primary" {
				t.Errorf("expected variant Primary, got %q", s.Variant)
			}
		}
	}
	if !foundButtonPrimary {
		t.Error("ui-button--primary not found in fixture")
	}
}

func TestFindImport(t *testing.T) {
	src := `import type { Meta } from '@storybook/react';
import { Button } from '../components/ui/button';
import { Card, CardHeader } from "@/components/ui/card";`

	tests := []struct {
		component  string
		wantSymbol string
		wantFrom   string
	}{
		{"Button", "Button", "../components/ui/button"},
		{"Card", "Card", "@/components/ui/card"},
		{"Missing", "", ""},
	}
	for _, tt := range tests {
		sym, from := findImport(src, tt.component)
		if sym != tt.wantSymbol || from != tt.wantFrom {
			t.Errorf("findImport(%q) = (%q,%q), want (%q,%q)",
				tt.component, sym, from, tt.wantSymbol, tt.wantFrom)
		}
	}
}

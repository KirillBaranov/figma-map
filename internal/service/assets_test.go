package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/figma"
)

// TestExportAssets_UsesExportSettings covers Phase 5: when the node has a
// designer-defined export preset and no explicit format is requested, that
// preset's format/scale is used instead of the hardcoded SVG/2 default.
func TestExportAssets_UsesExportSettings(t *testing.T) {
	scale := 3.0
	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		png:   tinyPNG(t),
		nodes: map[string]*figma.Node{
			"1:1": {
				ID: "1:1", Name: "Icon", Type: "VECTOR",
				ExportSettings: []figma.ExportSetting{
					{Format: "PNG", ConstraintType: "SCALE", ConstraintValue: &scale},
				},
			},
		},
	}
	s := &Service{cfg: config.Config{}, src: fake}

	outDir := t.TempDir()
	res, err := s.ExportAssets(context.Background(), "k", "1:1", "", outDir)
	if err != nil {
		t.Fatalf("ExportAssets: %v", err)
	}
	if res.Format != "PNG" {
		t.Errorf("format = %q, want PNG (from exportSettings)", res.Format)
	}
	if filepath.Ext(res.Path) != ".png" {
		t.Errorf("path = %q, want .png extension", res.Path)
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("exported file missing: %v", err)
	}
}

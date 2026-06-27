package service

import (
	"context"
	"os"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/figma"
)

// TestScreenshot_DefaultOutPath covers Phase 7: omitting --out still writes a
// file (under .figma-map/out/) and returns its path, and PNG bytes are only
// populated when inline=true — so an agent never gets flooded with base64
// unless it explicitly asks.
func TestScreenshot_DefaultOutPath(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		png:   tinyPNG(t),
	}
	s := &Service{cfg: config.Config{}, src: fake}

	res, err := s.Screenshot(context.Background(), "k", "1:1", 0, "", false)
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if res.Path == "" {
		t.Fatal("expected a default path, got empty Path")
	}
	if res.PNG != nil {
		t.Errorf("PNG should be nil without --inline, got %d bytes", len(res.PNG))
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("default-path file missing: %v", err)
	}

	inlineRes, err := s.Screenshot(context.Background(), "k", "1:1", 0, "", true)
	if err != nil {
		t.Fatalf("Screenshot inline: %v", err)
	}
	if len(inlineRes.PNG) == 0 {
		t.Error("expected PNG bytes with --inline")
	}
}

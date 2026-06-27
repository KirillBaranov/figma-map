package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// ExportResult reports an exported asset file.
type ExportResult struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
	Format string `json:"format"`
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
}

var assetFormats = map[string]string{"PNG": ".png", "SVG": ".svg", "JPG": ".jpg"}

// ExportAssets exports a node to outDir in the given format (PNG/SVG/JPG).
// Production-asset export (e.g. hero images, icons): export, don't regenerate.
// Deterministic (no API key).
func (s *Service) ExportAssets(ctx context.Context, fileKey, nodeID, format, outDir string) (ExportResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return ExportResult{}, err
	}
	node, err := s.src.Node(ctx, key, nodeID)
	if err != nil {
		return ExportResult{}, err
	}

	scale := 2.0
	format = strings.ToUpper(format)
	if format == "" {
		// Prefer the designer's own export preset over guessing — see
		// figma.ExportSetting.
		if len(node.ExportSettings) > 0 {
			preset := node.ExportSettings[0]
			format = strings.ToUpper(preset.Format)
			if preset.ConstraintType == "SCALE" && preset.ConstraintValue != nil {
				scale = *preset.ConstraintValue
			}
		} else {
			format = "SVG"
		}
	}
	ext, ok := assetFormats[format]
	if !ok {
		return ExportResult{}, fmt.Errorf("unsupported format %q (use PNG, SVG, or JPG)", format)
	}

	data, err := s.src.Screenshot(ctx, key, nodeID, figma.ScreenshotOpts{Format: format, Scale: scale})
	if err != nil {
		return ExportResult{}, err
	}

	if outDir == "" {
		outDir = "assets"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return ExportResult{}, err
	}
	path := filepath.Join(outDir, safeFileName(node.Name)+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{NodeID: nodeID, Name: node.Name, Format: format, Path: path, Bytes: len(data)}, nil
}

var nonFileChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func safeFileName(name string) string {
	s := nonFileChars.ReplaceAllString(name, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "asset"
	}
	return strings.ToLower(s)
}

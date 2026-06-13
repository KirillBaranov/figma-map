package service

import (
	"bytes"
	"context"
	"image"
	_ "image/png" // register PNG decoder for dimension probing
	"os"
	"path/filepath"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// ScreenshotResult carries the rendered image. PNG is excluded from JSON (it is
// surfaced as MCP ImageContent or written to a file); the CLI reports the path.
type ScreenshotResult struct {
	NodeID string `json:"nodeId"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Path   string `json:"path,omitempty"`
	PNG    []byte `json:"-"`
}

// Screenshot renders a node to PNG. If outPath is set the bytes are written
// there. Deterministic (no API key).
func (s *Service) Screenshot(_ context.Context, fileKey, nodeID string, scale float64, outPath string) (ScreenshotResult, error) {
	key, err := s.resolveFileKey(fileKey)
	if err != nil {
		return ScreenshotResult{}, err
	}
	if scale <= 0 {
		scale = 2
	}
	png, err := s.bridge.Screenshot(key, nodeID, figma.ScreenshotOpts{Format: "PNG", Scale: scale})
	if err != nil {
		return ScreenshotResult{}, err
	}

	res := ScreenshotResult{NodeID: nodeID, PNG: png}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(png)); err == nil {
		res.Width, res.Height = cfg.Width, cfg.Height
	}
	if outPath != "" {
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return res, err
		}
		if err := os.WriteFile(outPath, png, 0o644); err != nil {
			return res, err
		}
		res.Path = outPath
	}
	return res, nil
}

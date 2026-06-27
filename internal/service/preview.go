package service

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/render"
)

// RenderResult is a standalone screenshot of a Figma node's raw codegen
// output (no UIKit binding, no agent edits), rendered headless without
// needing a running app — see Render.
type RenderResult struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Path   string `json:"path,omitempty"`
	PNG    []byte `json:"-"`
}

// Render generates standalone HTML directly from a node's Figma tree (the
// same CSS computation Codegen uses, but real style="" attributes and no
// UIKit component substitution, since there's no app to mount one in) and
// screenshots it headless. The PNG is written to outPath (or, if empty, a
// default path under .figma-map/out/ — see defaultOutPath); pass inline=true
// to also populate RenderResult.PNG for the rare case the caller genuinely
// wants the bytes in the response.
//
// This exists for two things: sanity-checking the converter's own CSS output
// without first writing a Storybook story or test page, and as the
// "should-be" side of PixelDiff before there's a real implementation URL to
// compare against.
func (s *Service) Render(ctx context.Context, fileKey, nodeID string, scale float64, outPath string, inline bool) (RenderResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return RenderResult{}, err
	}
	if scale <= 0 {
		scale = 1
	}
	node, err := s.src.Node(ctx, key, nodeID)
	if err != nil {
		return RenderResult{}, err
	}

	w := int(math.Round(node.Bounds.Width))
	h := int(math.Round(node.Bounds.Height))

	png, err := render.ScreenshotHTML(previewHTML(node), w, h, scale)
	if err != nil {
		return RenderResult{}, fmt.Errorf("render preview: %w", err)
	}

	res := RenderResult{NodeID: node.ID, Name: node.Name, Width: w, Height: h}
	if inline {
		res.PNG = png
	}
	if outPath == "" {
		outPath = defaultOutPath(nodeID, "render", ".png")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(outPath, png, 0o644); err != nil {
		return res, err
	}
	res.Path = outPath
	return res, nil
}

// previewHTML renders node's subtree as a standalone HTML document sized to
// its exact Figma bounds.
func previewHTML(node *figma.Node) string {
	// node.Bounds.X/Y are absolute canvas coordinates; codeGen would place
	// the root div at that offset (correct for codegen output meant to live
	// inside its original canvas), which here would push it outside this
	// document's own viewport. Children keep their parent-relative bounds,
	// so zeroing only the root is enough.
	root := *node
	root.Bounds.X, root.Bounds.Y = 0, 0

	gen := &codeGen{html: true}
	body := gen.node(&root, 2, false, root.Bounds)
	w, h := node.Bounds.Width, node.Bounds.Height
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>* { margin: 0; padding: 0; box-sizing: border-box; } body { width: %gpx; height: %gpx; background: #fff; }</style>
</head>
<body>
<div style="position: relative; width: %gpx; height: %gpx;">
%s
</div>
</body>
</html>
`, w, h, w, h, body)
}

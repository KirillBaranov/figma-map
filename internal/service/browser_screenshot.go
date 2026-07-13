package service

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"

	"github.com/kirillbaranov/figma-map/internal/render"
)

// BrowserScreenshotResult is a standalone screenshot of a live URL — the
// implementation side of what `verify pixeldiff` compares against, but
// without a Figma node to diff against. Use it to just look at what's
// currently rendered (e.g. before a Figma render/binding even exists, or to
// hand a crop to something other than pixeldiff).
type BrowserScreenshotResult struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Path   string `json:"path,omitempty"`
	PNG    []byte `json:"-"`
}

// BrowserScreenshot renders url and returns a PNG — the whole viewport, or,
// with selector set, just the element matching it (a CSS selector, or a
// bare Figma node id expanded to `[data-figma-node="<id>"]`) cropped out of
// a normally-sized page viewport. Always writes to outPath (or a default
// path under .figma-map/out/) so the caller gets a path back; pass
// inline=true to also get the bytes in the response.
func (s *Service) BrowserScreenshot(ctx context.Context, url, selector string, width int, scale float64, outPath string, inline bool) (BrowserScreenshotResult, error) {
	if scale <= 0 {
		scale = 1
	}
	if width <= 0 {
		width = 1280
	}

	var png []byte
	var err error
	if selector != "" {
		targetURL, resolvedSelector, rerr := resolveScreenshotTarget(url, selector)
		if rerr != nil {
			return BrowserScreenshotResult{}, rerr
		}
		png, err = render.ScreenshotElement(ctx, targetURL, resolvedSelector, width, 900, scale)
	} else {
		png, err = render.Screenshot(ctx, url, width)
	}
	if err != nil {
		return BrowserScreenshotResult{}, err
	}

	res := BrowserScreenshotResult{}
	if inline {
		res.PNG = png
	}
	if cfg, _, derr := image.DecodeConfig(bytes.NewReader(png)); derr == nil {
		res.Width, res.Height = cfg.Width, cfg.Height
	}

	if outPath == "" {
		key := selector
		if key == "" {
			key = "page"
		}
		outPath = defaultOutPath(key, "browser-screenshot", ".png")
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

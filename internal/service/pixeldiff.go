package service

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/render"
)

// figmaNodeIDPattern matches a bare Figma node id — "1232:33509" or an
// instance-child id like "I1232:33509;260:2268" — as opposed to an actual
// CSS selector. Node ids contain ':', which collides with a naive
// "does it look like CSS" character check (':' also introduces a
// pseudo-class), so Selector expansion matches this pattern explicitly
// rather than guessing from punctuation.
var figmaNodeIDPattern = regexp.MustCompile(`^I?\d+:\d+(;\d+:\d+)*$`)

// resolveScreenshotTarget prepares a (url, selector) pair for
// render.ScreenshotElement: expands a bare Figma node id into its
// data-figma-node attribute selector, and resolves a schemeless url to a
// file:// URL (same rule other browser-screenshot paths in this file use)
// so a relative local HTML path works the same way with or without
// --selector.
func resolveScreenshotTarget(url, selector string) (targetURL, resolvedSelector string, err error) {
	resolvedSelector = selector
	if figmaNodeIDPattern.MatchString(selector) {
		resolvedSelector = fmt.Sprintf(`[data-figma-node="%s"]`, selector)
	}
	targetURL = url
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "file://") {
		abs, aerr := filepath.Abs(url)
		if aerr != nil {
			return "", "", fmt.Errorf("resolve html path %q: %w", url, aerr)
		}
		targetURL = "file://" + abs
	}
	return targetURL, resolvedSelector, nil
}

// PixelDiffResult is the result of a pixel-level screenshot comparison.
type PixelDiffResult struct {
	Match      bool    `json:"match"`
	DiffPct    float64 `json:"diffPct"`
	DiffPixels int     `json:"diffPixels"`
	Total      int     `json:"total"`
	Threshold  float64 `json:"threshold"`
	// DiffOut is the path where the diff image was saved, if requested.
	DiffOut string `json:"diffOut,omitempty"`
	// Regions is a fixed-grid breakdown of diff% per cell, sorted worst
	// first — a textual "where to look" signal, no image interpretation
	// needed. Empty when GridSize<=0.
	Regions []DiffRegion `json:"regions,omitempty"`
	// Clusters are real connected-component diff regions, classified by
	// likely cause (attributed pixeldiff — see render.Cluster). Only
	// populated when PixelDiffOptions.Cluster is set. Additive alongside
	// Regions, not a replacement.
	Clusters []Cluster `json:"clusters,omitempty"`
	// Issues mirrors Clusters into the cascade's unified issue-list shape
	// (see issue.go) — same additive relationship Reconcile's Diff.Issues
	// has to ByElement.
	Issues []Issue `json:"issues,omitempty"`
}

// DiffRegion is one fixed-grid cell's diff percentage.
type DiffRegion struct {
	X       int     `json:"x"`
	Y       int     `json:"y"`
	W       int     `json:"w"`
	H       int     `json:"h"`
	DiffPct float64 `json:"diffPct"`
}

// Cluster is one connected-component diff region, classified by likely
// cause — see render.Cluster for the algorithm.
type Cluster struct {
	X          int `json:"x"`
	Y          int `json:"y"`
	W          int `json:"w"`
	H          int `json:"h"`
	DiffPixels int `json:"diffPixels"`
	// Kind is "shift", "color", or "other" — see render.Cluster.
	Kind    string `json:"kind"`
	OffsetX int    `json:"offsetX,omitempty"`
	OffsetY int    `json:"offsetY,omitempty"`
}

// PixelDiffOptions controls the comparison.
type PixelDiffOptions struct {
	// Threshold is the maximum diffPct before Match=false. Default 5.0.
	Threshold float64
	// ColorTol is the per-channel tolerance (0–255). Default 10.
	ColorTol uint8
	// DiffOut is the path to write the annotated diff PNG. Empty = skip.
	DiffOut string
	// Scale is the deviceScaleFactor for the browser screenshot. Default 1.
	Scale float64
	// GridSize buckets the comparison into a GridSize×GridSize grid for
	// Regions. Default 4; pass a negative value to disable region computation.
	GridSize int
	// Cluster additionally computes connected-component Clusters/Issues
	// (attributed pixeldiff) — costs more than the grid alone, so opt-in.
	Cluster bool
	// Selector scopes the implementation-side screenshot to one element
	// instead of the whole viewport/page — a CSS selector, or a bare Figma
	// node id (expanded to `[data-figma-node="<id>"]`). Lets the agent diff
	// a section that lives mid-page (not rendered as its own isolated
	// story/HTML) against its Figma render. Only applies to the url
	// screenshot path; ignored when url == "" (own codegen render already
	// renders the node in isolation).
	Selector string
	// Width is the browser viewport width in CSS px, used only when
	// Selector is set. Defaults to 1280 rather than the Figma node's own
	// width: a scoped section's layout is usually driven by the width of
	// the page/container it lives in, not by its own size, unlike the
	// isolated-story case which renders at exactly the node's width.
	Width int
}

// PixelDiff takes a Figma screenshot and a browser screenshot, then compares
// them pixel-by-pixel. url selects what the browser screenshots:
//
//   - empty: no implementation to compare against yet — render the node's raw
//     codegen output (see Render) and diff against that. Useful for
//     sanity-checking the converter itself before there's a real component.
//   - a local path (no scheme): a static HTML file, screenshotted directly via
//     file:// — no server required. For a hand- or agent-adapted preview.
//   - http(s):// or file://: an already-running implementation (dev server,
//     Storybook iframe, etc.) rendering the component in isolation so both
//     images cover the same region.
//
// The browser viewport is set to the Figma node's exact CSS dimensions.
func (s *Service) PixelDiff(ctx context.Context, fileKey, nodeID, url string, opts PixelDiffOptions) (PixelDiffResult, error) {
	if opts.Threshold <= 0 {
		opts.Threshold = 5.0
	}
	if opts.ColorTol == 0 {
		opts.ColorTol = 10
	}
	if opts.Scale <= 0 {
		opts.Scale = 1
	}
	if opts.GridSize == 0 {
		opts.GridSize = 4
	} else if opts.GridSize < 0 {
		opts.GridSize = 0
	}

	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return PixelDiffResult{}, err
	}

	// 1. Get Figma node to know the dimensions.
	node, err := s.src.Node(ctx, key, nodeID)
	if err != nil {
		return PixelDiffResult{}, err
	}

	w := int(math.Round(node.Bounds.Width))
	h := int(math.Round(node.Bounds.Height))

	// 2. Figma screenshot at scale=1 (so pixel dims = CSS dims).
	figmaPNG, err := s.src.Screenshot(ctx, key, nodeID, figma.ScreenshotOpts{
		Format: "PNG",
		Scale:  opts.Scale,
	})
	if err != nil {
		return PixelDiffResult{}, fmt.Errorf("figma screenshot: %w", err)
	}

	// 3. Browser screenshot at the same viewport size (or, with a Selector,
	// one element cropped out of a normally-sized page viewport).
	var browserPNG []byte
	switch {
	case url == "":
		browserPNG, err = render.ScreenshotHTML(previewHTML(node), w, h, opts.Scale)
	case opts.Selector != "":
		targetURL, selector, rerr := resolveScreenshotTarget(url, opts.Selector)
		if rerr != nil {
			return PixelDiffResult{}, rerr
		}
		vw := opts.Width
		if vw <= 0 {
			vw = 1280
		}
		browserPNG, err = render.ScreenshotElement(ctx, targetURL, selector, vw, 900, opts.Scale)
	case strings.HasPrefix(url, "http://"), strings.HasPrefix(url, "https://"), strings.HasPrefix(url, "file://"):
		browserPNG, err = render.ScreenshotViewport(url, w, h, opts.Scale)
	default:
		abs, aerr := filepath.Abs(url)
		if aerr != nil {
			return PixelDiffResult{}, fmt.Errorf("resolve html path %q: %w", url, aerr)
		}
		browserPNG, err = render.ScreenshotViewport("file://"+abs, w, h, opts.Scale)
	}
	if err != nil {
		return PixelDiffResult{}, fmt.Errorf("browser screenshot: %w", err)
	}

	// 4. Pixel diff.
	produceDiff := opts.DiffOut != ""
	diff, err := render.PixelDiff(figmaPNG, browserPNG, opts.ColorTol, produceDiff, opts.GridSize, opts.Cluster)
	if err != nil {
		return PixelDiffResult{}, fmt.Errorf("pixel diff: %w", err)
	}

	result := PixelDiffResult{
		Match:      diff.DiffPct <= opts.Threshold,
		DiffPct:    math.Round(diff.DiffPct*100) / 100,
		DiffPixels: diff.DiffPixels,
		Total:      diff.TotalPixels,
		Threshold:  opts.Threshold,
	}
	for _, r := range diff.Regions {
		result.Regions = append(result.Regions, DiffRegion{
			X: r.X, Y: r.Y, W: r.W, H: r.H,
			DiffPct: math.Round(r.DiffPct*100) / 100,
		})
	}
	for _, c := range diff.Clusters {
		result.Clusters = append(result.Clusters, Cluster{
			X: c.X, Y: c.Y, W: c.W, H: c.H,
			DiffPixels: c.DiffPixels, Kind: c.Kind, OffsetX: c.OffsetX, OffsetY: c.OffsetY,
		})
	}
	result.Issues = IssuesFromClusters(result.Clusters)

	if produceDiff && len(diff.DiffImage) > 0 {
		if err := os.WriteFile(opts.DiffOut, diff.DiffImage, 0o644); err != nil {
			return result, fmt.Errorf("write diff image: %w", err)
		}
		result.DiffOut = opts.DiffOut
	}

	return result, nil
}

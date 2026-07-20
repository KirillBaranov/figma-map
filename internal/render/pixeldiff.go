package render

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sort"
)

// DiffResult holds the outcome of a pixel-level comparison.
type DiffResult struct {
	// DiffPct is the percentage of pixels that differ beyond the color threshold.
	DiffPct float64
	// DiffPixels is the raw count of differing pixels.
	DiffPixels int
	// TotalPixels is the number of pixels compared (min of both image areas).
	TotalPixels int
	// DiffImage is a PNG-encoded diff image: green where images match,
	// red-tinted where they differ (intensity scales with difference).
	// Only produced when produceDiff=true.
	DiffImage []byte
	// Regions is a fixed gridSize×gridSize breakdown of DiffPct per cell,
	// sorted descending — a coarse "where to look" signal that's plain
	// numbers, not an image a vision model has to interpret. Arithmetic
	// bucketing only, not shape/cluster inference. Empty when gridSize<=0.
	Regions []Region
	// Clusters are real connected-component regions of the diff mask,
	// classified by likely cause (shift/color/other) — see Cluster. Unlike
	// Regions, these follow the actual shape of what differs and try to say
	// why, not just where. Only computed when cluster=true (see PixelDiff).
	Clusters []Cluster
}

// Region is one fixed-grid cell's diff percentage.
type Region struct {
	X, Y, W, H int
	DiffPct    float64
}

// PixelDiff compares two PNG images pixel-by-pixel. colorTol is the per-channel
// tolerance (0–255); pixels within tolerance on all channels count as matching.
// Set produceDiff=true to get an annotated diff image in the result. gridSize
// buckets the comparison into a gridSize×gridSize grid for DiffResult.Regions;
// 0 skips region computation. cluster=true additionally computes
// DiffResult.Clusters (connected-component regions, classified by likely
// cause) — opt-in since clustering costs more than the grid bucketing.
func PixelDiff(refPNG, gotPNG []byte, colorTol uint8, produceDiff bool, gridSize int, cluster bool) (DiffResult, error) {
	refImg, err := decodePNG(refPNG)
	if err != nil {
		return DiffResult{}, fmt.Errorf("decode reference image: %w", err)
	}
	gotImg, err := decodePNG(gotPNG)
	if err != nil {
		return DiffResult{}, fmt.Errorf("decode implementation image: %w", err)
	}

	// Normalize both to RGBA so pixel reads are uniform.
	ref := toRGBA(refImg)
	got := toRGBA(gotImg)

	// Compare over the intersection of both image bounds.
	rBounds := ref.Bounds()
	gBounds := got.Bounds()
	w := min(rBounds.Max.X, gBounds.Max.X)
	h := min(rBounds.Max.Y, gBounds.Max.Y)

	var diffImg *image.RGBA
	if produceDiff {
		diffImg = image.NewRGBA(image.Rect(0, 0, w, h))
	}

	var mask []bool
	if cluster {
		mask = make([]bool, w*h)
	}

	total := w * h
	diffCount := 0

	// Per-cell counters for the Regions breakdown, indexed [row*gridSize+col].
	var cellDiff, cellTotal []int
	if gridSize > 0 {
		cellDiff = make([]int, gridSize*gridSize)
		cellTotal = make([]int, gridSize*gridSize)
	}
	cellOf := func(x, y int) int {
		col := x * gridSize / w
		row := y * gridSize / h
		if col >= gridSize {
			col = gridSize - 1
		}
		if row >= gridSize {
			row = gridSize - 1
		}
		return row*gridSize + col
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rPx := ref.RGBAAt(x, y)
			gPx := got.RGBAAt(x, y)

			dr := absDiff(rPx.R, gPx.R)
			dg := absDiff(rPx.G, gPx.G)
			db := absDiff(rPx.B, gPx.B)
			maxCh := max8(dr, dg, db)

			var cell int
			if gridSize > 0 {
				cell = cellOf(x, y)
				cellTotal[cell]++
			}

			if maxCh > colorTol {
				diffCount++
				if gridSize > 0 {
					cellDiff[cell]++
				}
				if mask != nil {
					mask[y*w+x] = true
				}
				if diffImg != nil {
					// Red tint, brighter for larger differences.
					intensity := uint8(math.Min(255, float64(maxCh)*2))
					diffImg.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: intensity})
				}
			} else if diffImg != nil {
				// Slightly dim matching pixels to make diffs pop.
				diffImg.SetRGBA(x, y, color.RGBA{
					R: rPx.R / 2,
					G: rPx.G / 2,
					B: rPx.B / 2,
					A: 128,
				})
			}
		}
	}

	result := DiffResult{
		DiffPixels:  diffCount,
		TotalPixels: total,
	}
	if total > 0 {
		result.DiffPct = float64(diffCount) / float64(total) * 100
	}

	if gridSize > 0 {
		result.Regions = buildRegions(gridSize, w, h, cellDiff, cellTotal)
	}

	if mask != nil {
		result.Clusters = clusterAndClassify(ref, got, mask, w, h, colorTol)
	}

	if diffImg != nil {
		var buf bytes.Buffer
		if err := png.Encode(&buf, diffImg); err != nil {
			return result, fmt.Errorf("encode diff image: %w", err)
		}
		result.DiffImage = buf.Bytes()
	}

	return result, nil
}

// ScreenshotViewport opens a new tab, sets the viewport to exactly w×h CSS px
// at deviceScaleFactor=scale, and returns a PNG. Use scale=1 to match Figma
// screenshots taken with scale=1 so dimensions align before diffing.
func ScreenshotViewport(url string, w, h int, scale float64) ([]byte, error) {
	return screenshotViewport(url, w, h, scale)
}

// CropPNG decodes a PNG, crops it to (x, y, w, h) in pixels (clamped to the
// image bounds), and re-encodes. Used to scope a full-frame/full-page
// screenshot down to one region — e.g. the VLM tier sending only an
// unresolved crop instead of the whole image (see reconcile_tier2.go).
func CropPNG(data []byte, x, y, w, h int) ([]byte, error) {
	img, err := decodePNG(data)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	b := img.Bounds()
	x = clampInt(x, 0, b.Max.X)
	y = clampInt(y, 0, b.Max.Y)
	// A requested region entirely outside the image (e.g. a crop rect from
	// a different coordinate space than expected) would otherwise clamp to
	// a 0×0 box, which the PNG encoder rejects — floor at 1px so cropping
	// never errors on a bad-but-plausible region, it just returns
	// (almost) nothing useful, which the caller's VLM call can still parse.
	w = max(1, clampInt(w, 0, b.Max.X-x))
	h = max(1, clampInt(h, 0, b.Max.Y-y))

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), img, image.Pt(x, y), draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return buf.Bytes(), nil
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func decodePNG(data []byte) (image.Image, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}

// buildRegions turns the per-cell counters into a Regions slice, sorted by
// DiffPct descending so the worst cell is first.
func buildRegions(gridSize, w, h int, cellDiff, cellTotal []int) []Region {
	regions := make([]Region, 0, gridSize*gridSize)
	for row := 0; row < gridSize; row++ {
		for col := 0; col < gridSize; col++ {
			i := row*gridSize + col
			if cellTotal[i] == 0 {
				continue
			}
			x0 := col * w / gridSize
			y0 := row * h / gridSize
			x1 := (col + 1) * w / gridSize
			y1 := (row + 1) * h / gridSize
			regions = append(regions, Region{
				X: x0, Y: y0, W: x1 - x0, H: y1 - y0,
				DiffPct: float64(cellDiff[i]) / float64(cellTotal[i]) * 100,
			})
		}
	}
	sort.Slice(regions, func(i, j int) bool { return regions[i].DiffPct > regions[j].DiffPct })
	return regions
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func max8(a, b, c uint8) uint8 {
	if a >= b && a >= c {
		return a
	}
	if b >= c {
		return b
	}
	return c
}

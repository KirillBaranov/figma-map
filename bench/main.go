// Command bench scores how close an implementation is to a Figma design using
// an INDEPENDENT metric (pixel diff), so it does not favor figma-map's own
// reconcile oracle. Given a design PNG and one or more rendered implementation
// URLs, it renders each, computes the share of differing pixels, and writes a
// side-by-side composite plus per-arm diff heatmaps.
//
// Usage:
//
//	bench -design design.png -width 1440 -arm baseline=http://localhost:8201/ \
//	      -arm treatment=http://localhost:8202/ -out bench/out
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/render"
)

type arm struct {
	name string
	url  string
}

type armFlags []arm

func (a *armFlags) String() string { return "" }
func (a *armFlags) Set(v string) error {
	name, url, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("arm must be name=url, got %q", v)
	}
	*a = append(*a, arm{name: name, url: url})
	return nil
}

func main() {
	var (
		design = flag.String("design", "", "design reference PNG")
		width  = flag.Int("width", 1440, "render width (the Figma frame width)")
		out    = flag.String("out", "bench/out", "output directory")
		arms   armFlags
	)
	flag.Var(&arms, "arm", "implementation arm as name=url (repeatable)")
	flag.Parse()

	if *design == "" || len(arms) == 0 {
		fmt.Fprintln(os.Stderr, "need -design and at least one -arm name=url")
		os.Exit(2)
	}
	if err := run(*design, *width, *out, arms); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(designPath string, width int, outDir string, arms []arm) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	designImg, err := loadPNG(designPath)
	if err != nil {
		return fmt.Errorf("load design: %w", err)
	}
	dw, dh := designImg.Bounds().Dx(), designImg.Bounds().Dy()

	type result struct {
		arm     arm
		shot    image.Image
		diffPct float64
	}
	var results []result

	for _, a := range arms {
		fmt.Printf("Rendering %s (%s) …\n", a.name, a.url)
		pngBytes, err := render.Screenshot(context.Background(), a.url, width)
		if err != nil {
			return fmt.Errorf("render %s: %w", a.name, err)
		}
		shot, err := decodePNG(pngBytes)
		if err != nil {
			return err
		}
		shot = cropTo(shot, dw, dh)

		pct, heat := pixelDiff(designImg, shot)
		if err := savePNG(filepath.Join(outDir, a.name+"_diff.png"), heat); err != nil {
			return err
		}
		if err := savePNG(filepath.Join(outDir, a.name+".png"), shot); err != nil {
			return err
		}
		results = append(results, result{arm: a, shot: shot, diffPct: pct})
		fmt.Printf("  %s: %.2f%% pixels differ from design\n", a.name, pct)
	}

	// Side-by-side: design + each arm.
	panels := []image.Image{designImg}
	labels := []string{"design"}
	for _, r := range results {
		panels = append(panels, r.shot)
		labels = append(labels, r.arm.name)
	}
	if err := savePNG(filepath.Join(outDir, "sidebyside.png"), sideBySide(panels)); err != nil {
		return err
	}

	fmt.Printf("\nPixel-diff vs design (lower is closer):\n")
	for _, r := range results {
		fmt.Printf("  %-12s %.2f%%\n", r.arm.name, r.diffPct)
	}
	fmt.Printf("Wrote composite + heatmaps to %s (panels: %s)\n", outDir, strings.Join(labels, " | "))
	return nil
}

// pixelDiff returns the percentage of pixels that differ beyond a tolerance and
// a heatmap (differing pixels in red over a dimmed design).
func pixelDiff(a, b image.Image) (float64, image.Image) {
	const tol = 100 // summed per-channel difference to count as "different"
	w := min(a.Bounds().Dx(), b.Bounds().Dx())
	h := min(a.Bounds().Dy(), b.Bounds().Dy())
	heat := image.NewRGBA(image.Rect(0, 0, w, h))

	var diff int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ar, ag, ab, _ := a.At(a.Bounds().Min.X+x, a.Bounds().Min.Y+y).RGBA()
			br, bg, bb, _ := b.At(b.Bounds().Min.X+x, b.Bounds().Min.Y+y).RGBA()
			d := abs8(ar, br) + abs8(ag, bg) + abs8(ab, bb)
			if d > tol {
				diff++
				heat.Set(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				// dimmed grayscale of the design for context
				g := uint8((ar >> 8) / 3)
				heat.Set(x, y, color.RGBA{g, g, g, 255})
			}
		}
	}
	total := w * h
	if total == 0 {
		return 0, heat
	}
	return float64(diff) / float64(total) * 100, heat
}

// abs8 returns the absolute difference of two 16-bit channel values, scaled to
// 0..255.
func abs8(a, b uint32) int {
	x := int(a>>8) - int(b>>8)
	if x < 0 {
		x = -x
	}
	return x
}

func cropTo(img image.Image, w, h int) image.Image {
	b := img.Bounds()
	cw, ch := min(w, b.Dx()), min(h, b.Dy())
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, image.Rect(0, 0, w, h), &image.Uniform{color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(0, 0, cw, ch), img, b.Min, draw.Src)
	return dst
}

func sideBySide(imgs []image.Image) image.Image {
	const gap = 16
	var w, h int
	for _, im := range imgs {
		w += im.Bounds().Dx() + gap
		h = max(h, im.Bounds().Dy())
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.RGBA{30, 30, 30, 255}}, image.Point{}, draw.Src)
	x := 0
	for _, im := range imgs {
		draw.Draw(dst, image.Rect(x, 0, x+im.Bounds().Dx(), im.Bounds().Dy()), im, im.Bounds().Min, draw.Src)
		x += im.Bounds().Dx() + gap
	}
	return dst
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return png.Decode(f)
}

func decodePNG(b []byte) (image.Image, error) { return png.Decode(strings.NewReader(string(b))) }

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

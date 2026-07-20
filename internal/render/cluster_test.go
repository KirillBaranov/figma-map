package render

import (
	"image"
	"image/color"
	"testing"
)

// solidRect draws a filled rectangle of fg on a bg-filled canvas.
func solidRect(w, h, x0, y0, rw, rh int, bg, fg color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, bg)
		}
	}
	for y := y0; y < y0+rh; y++ {
		for x := x0; x < x0+rw; x++ {
			img.SetRGBA(x, y, fg)
		}
	}
	return img
}

func TestPixelDiff_ClusterDetectsShift(t *testing.T) {
	// A 10x10 black square, in ref at (20,20), in got shifted to (25,23) —
	// a real translation, well within the search radius.
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}
	ref := solidRect(80, 80, 20, 20, 10, 10, white, black)
	got := solidRect(80, 80, 25, 23, 10, 10, white, black)

	result, err := PixelDiff(encodePNG(t, ref), encodePNG(t, got), 10, false, 0, true)
	if err != nil {
		t.Fatalf("PixelDiff: %v", err)
	}
	if len(result.Clusters) == 0 {
		t.Fatal("expected at least one cluster")
	}
	c := result.Clusters[0]
	if c.Kind != "shift" {
		t.Fatalf("expected Kind=shift, got %+v", c)
	}
	if c.OffsetX != 5 || c.OffsetY != 3 {
		t.Errorf("expected offset (5,3), got (%d,%d)", c.OffsetX, c.OffsetY)
	}
}

func TestPixelDiff_ClusterDetectsColor(t *testing.T) {
	// Same position and size, only the color changed — no translation
	// improves the match, so this must classify as "color", not "shift".
	white := color.RGBA{255, 255, 255, 255}
	red := color.RGBA{220, 20, 20, 255}
	blue := color.RGBA{20, 20, 220, 255}
	ref := solidRect(80, 80, 20, 20, 20, 20, white, red)
	got := solidRect(80, 80, 20, 20, 20, 20, white, blue)

	result, err := PixelDiff(encodePNG(t, ref), encodePNG(t, got), 10, false, 0, true)
	if err != nil {
		t.Fatalf("PixelDiff: %v", err)
	}
	if len(result.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %+v", len(result.Clusters), result.Clusters)
	}
	if result.Clusters[0].Kind != "color" {
		t.Errorf("expected Kind=color, got %+v", result.Clusters[0])
	}
}

func TestPixelDiff_NoClustersWhenDisabled(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}
	ref := solidRect(40, 40, 5, 5, 10, 10, white, black)
	got := solidRect(40, 40, 8, 8, 10, 10, white, black)

	result, err := PixelDiff(encodePNG(t, ref), encodePNG(t, got), 10, false, 0, false)
	if err != nil {
		t.Fatalf("PixelDiff: %v", err)
	}
	if result.Clusters != nil {
		t.Errorf("expected nil Clusters when cluster=false, got %+v", result.Clusters)
	}
}

func TestPixelDiff_InducedDiffSubtraction(t *testing.T) {
	// Two squares, both shifted by the same (5,5) offset — a single global
	// shift explains both. Without induced-diff subtraction this would
	// report 2 "shift" clusters; with it, the smaller one (fully explained
	// by the same offset as the primary) should be absorbed.
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}

	w, h := 120, 120
	ref := image.NewRGBA(image.Rect(0, 0, w, h))
	got := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ref.SetRGBA(x, y, white)
			got.SetRGBA(x, y, white)
		}
	}
	drawRect := func(img *image.RGBA, x0, y0, rw, rh int) {
		for y := y0; y < y0+rh; y++ {
			for x := x0; x < x0+rw; x++ {
				img.SetRGBA(x, y, black)
			}
		}
	}
	// Primary: a large square, offset (5,5) between ref/got.
	drawRect(ref, 10, 10, 30, 30)
	drawRect(got, 15, 15, 30, 30)
	// Secondary: a small square elsewhere on the canvas, same (5,5) offset.
	drawRect(ref, 70, 70, 10, 10)
	drawRect(got, 75, 75, 10, 10)

	result, err := PixelDiff(encodePNG(t, ref), encodePNG(t, got), 10, false, 0, true)
	if err != nil {
		t.Fatalf("PixelDiff: %v", err)
	}
	if len(result.Clusters) != 1 {
		t.Fatalf("expected the secondary cluster to be absorbed by induced-diff subtraction, got %d clusters: %+v",
			len(result.Clusters), result.Clusters)
	}
	if result.Clusters[0].Kind != "shift" || result.Clusters[0].OffsetX != 5 || result.Clusters[0].OffsetY != 5 {
		t.Errorf("unexpected primary cluster: %+v", result.Clusters[0])
	}
}

func TestConnectedComponents_SeparatesDisjointRegions(t *testing.T) {
	w, h := 10, 10
	mask := make([]bool, w*h)
	set := func(x, y int) { mask[y*w+x] = true }
	// Two disjoint 2x2 blocks, far apart.
	set(0, 0)
	set(1, 0)
	set(0, 1)
	set(1, 1)
	set(8, 8)
	set(9, 8)
	set(8, 9)
	set(9, 9)

	comps := connectedComponents(mask, w, h)
	if len(comps) != 2 {
		t.Fatalf("expected 2 components, got %d: %+v", len(comps), comps)
	}
	for _, c := range comps {
		if c.diffPixels != 4 {
			t.Errorf("expected each component to have 4 pixels, got %+v", c)
		}
	}
}

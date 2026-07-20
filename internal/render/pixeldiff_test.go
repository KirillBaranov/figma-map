package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestPixelDiff_Regions covers Phase 9: a 4x4px image where only the
// top-left 2x2 quadrant differs should report a near-100% diff region there
// and ~0% everywhere else, with a 2x2 grid.
func TestPixelDiff_Regions(t *testing.T) {
	const size = 4
	ref := image.NewRGBA(image.Rect(0, 0, size, size))
	got := image.NewRGBA(image.Rect(0, 0, size, size))
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			ref.SetRGBA(x, y, white)
			if x < size/2 && y < size/2 {
				got.SetRGBA(x, y, black) // top-left quadrant differs
			} else {
				got.SetRGBA(x, y, white)
			}
		}
	}

	result, err := PixelDiff(encodePNG(t, ref), encodePNG(t, got), 10, false, 2, false)
	if err != nil {
		t.Fatalf("PixelDiff: %v", err)
	}
	if len(result.Regions) != 4 {
		t.Fatalf("want 4 regions, got %d", len(result.Regions))
	}
	// Worst region (sorted descending) must be the diffing quadrant.
	worst := result.Regions[0]
	if worst.DiffPct < 99 {
		t.Errorf("worst region diffPct = %.2f, want ~100", worst.DiffPct)
	}
	if worst.X != 0 || worst.Y != 0 {
		t.Errorf("worst region at (%d,%d), want top-left (0,0)", worst.X, worst.Y)
	}
	// Remaining regions should be clean.
	for _, r := range result.Regions[1:] {
		if r.DiffPct > 1 {
			t.Errorf("expected ~0%% diff outside top-left quadrant, got region %+v", r)
		}
	}
}

// TestPixelDiff_NoRegions covers gridSize<=0 skipping region computation.
func TestPixelDiff_NoRegions(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	result, err := PixelDiff(encodePNG(t, img), encodePNG(t, img), 10, false, 0, false)
	if err != nil {
		t.Fatalf("PixelDiff: %v", err)
	}
	if result.Regions != nil {
		t.Errorf("expected nil Regions with gridSize=0, got %+v", result.Regions)
	}
}

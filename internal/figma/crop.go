package figma

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
)

// cropPNG decodes a PNG, crops it to (x, y, w, h) in pixels, and re-encodes.
func cropPNG(data []byte, x, y, w, h int) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	b := img.Bounds()
	x = clampInt(x, 0, b.Max.X)
	y = clampInt(y, 0, b.Max.Y)
	w = clampInt(w, 0, b.Max.X-x)
	h = clampInt(h, 0, b.Max.Y-y)

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

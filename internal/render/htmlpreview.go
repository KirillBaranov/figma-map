package render

import (
	"fmt"
	"os"
	"path/filepath"
)

// ScreenshotHTML writes html to a temp file and screenshots it via a file://
// URL — no HTTP server required. Use for standalone previews and for
// pixeldiff when there's no running app yet to point a URL at.
func ScreenshotHTML(html string, w, h int, scale float64) ([]byte, error) {
	f, err := os.CreateTemp("", "figma-map-preview-*.html")
	if err != nil {
		return nil, fmt.Errorf("create temp html: %w", err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(html); err != nil {
		f.Close()
		return nil, fmt.Errorf("write temp html: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	abs, err := filepath.Abs(f.Name())
	if err != nil {
		return nil, err
	}
	return screenshotViewport("file://"+abs, w, h, scale)
}

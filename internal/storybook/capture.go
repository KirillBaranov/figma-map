package storybook

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

// FetchIndex downloads and returns the raw index.json from a running Storybook.
func FetchIndex(storybookURL string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(storybookURL + "/index.json")
	if err != nil {
		return nil, fmt.Errorf("fetch index.json from %s: %w", storybookURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("storybook index.json returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Capturer screenshots Storybook stories via a headless Chrome instance.
type Capturer struct {
	storybookURL string
	width        int64
	height       int64
}

// NewCapturer returns a Capturer for the given Storybook base URL.
func NewCapturer(storybookURL string) *Capturer {
	return &Capturer{storybookURL: storybookURL, width: 800, height: 600}
}

// CaptureAll screenshots every story into <dir>/png/<id>.png, sets each story's
// PNGPath, and returns. A single Chrome context is reused across stories.
func (c *Capturer) CaptureAll(ctx context.Context, stories []Story, dir string) error {
	pngDir := filepath.Join(dir, "png")
	if err := os.MkdirAll(pngDir, 0o755); err != nil {
		return err
	}

	allocCtx, cancelAlloc := chromedp.NewContext(ctx)
	defer cancelAlloc()
	// Force browser startup so the first story isn't charged the launch cost.
	if err := chromedp.Run(allocCtx); err != nil {
		return fmt.Errorf("launch headless chrome (is Chrome installed?): %w", err)
	}

	for i := range stories {
		s := &stories[i]
		png, err := c.captureOne(allocCtx, s.ID)
		if err != nil {
			return fmt.Errorf("capture %s: %w", s.ID, err)
		}
		rel := filepath.Join("png", s.ID+".png")
		if err := os.WriteFile(filepath.Join(dir, rel), png, 0o644); err != nil {
			return err
		}
		s.PNGPath = rel
	}
	return nil
}

// captureOne renders a single story's isolated iframe and returns PNG bytes.
func (c *Capturer) captureOne(ctx context.Context, storyID string) ([]byte, error) {
	url := fmt.Sprintf("%s/iframe.html?id=%s&viewMode=story", c.storybookURL, storyID)

	tabCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()
	tabCtx, cancelTimeout := context.WithTimeout(tabCtx, 20*time.Second)
	defer cancelTimeout()

	var buf []byte
	err := chromedp.Run(tabCtx,
		chromedp.EmulateViewport(c.width, c.height),
		chromedp.Navigate(url),
		chromedp.WaitReady("#storybook-root", chromedp.ByID),
		// Let fonts/styles settle before the shot.
		chromedp.Sleep(500*time.Millisecond),
		// Crop to the rendered story element so the component fills the frame
		// instead of floating in viewport whitespace — better for matching.
		chromedp.Screenshot("#storybook-root", &buf, chromedp.ByID),
	)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

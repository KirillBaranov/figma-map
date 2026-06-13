// Package render drives headless Chrome to render the agent's output and read
// the real, computed values from the DOM. These exact "is" values are the
// deterministic counterpart to the Figma "should-be" tokens, so reconcile
// compares numbers instead of guessing from pixels.
package render

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// One headless browser is shared across all renders; each call opens a fresh
// tab. This keeps an agent's reconcile loop from launching Chrome on every call.
var (
	browserOnce sync.Once
	browserCtx  context.Context
)

// tab opens a new tab on the shared browser, bounded by a timeout and cancelled
// if the caller's context is done. The returned cancel closes the tab (the
// browser itself is kept warm for the process lifetime).
func tab(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	browserOnce.Do(func() {
		alloc, _ := chromedp.NewExecAllocator(context.Background(), chromedp.DefaultExecAllocatorOptions[:]...)
		browserCtx, _ = chromedp.NewContext(alloc)
	})
	tabCtx, cancelTab := chromedp.NewContext(browserCtx)
	stop := context.AfterFunc(ctx, cancelTab)
	timed, cancelTimed := context.WithTimeout(tabCtx, timeout)
	return timed, func() { stop(); cancelTimed(); cancelTab() }
}

// Box is an element's layout rectangle in CSS pixels.
type Box struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// DOMElement is one rendered element. FigmaNode is the data-figma-node attribute
// when present (exact alignment); Text and Box support tag-free spatial
// alignment against an existing implementation.
type DOMElement struct {
	FigmaNode string            `json:"figmaNode"`
	Tag       string            `json:"tag"`
	Text      string            `json:"text,omitempty"`
	Styles    map[string]string `json:"styles"`
	Box       Box               `json:"box"`
}

// extractJS collects every visible, sized element's computed styles, box, own
// text, and data-figma-node attribute (when present). Returning all elements —
// not just tagged ones — lets reconcile align against untagged implementations.
const extractJS = `(() => {
  const props = ['background-color','color','font-size','font-weight','font-family',
    'line-height','letter-spacing','text-align','border-top-left-radius',
    'border-top-width','border-top-color','padding-top','padding-right',
    'padding-bottom','padding-left','gap','column-gap','row-gap','opacity'];
  const skip = new Set(['SCRIPT','STYLE','HEAD','META','LINK','TITLE','BR','NOSCRIPT','HTML']);
  const out = [];
  for (const el of document.querySelectorAll('*')) {
    if (skip.has(el.tagName)) continue;
    const r = el.getBoundingClientRect();
    if (r.width < 1 || r.height < 1) continue;
    const cs = getComputedStyle(el);
    if (cs.display === 'none' || cs.visibility === 'hidden') continue;
    const styles = {};
    props.forEach(p => { styles[p] = cs.getPropertyValue(p); });
    let text = '';
    for (const n of el.childNodes) { if (n.nodeType === 3) text += n.textContent; }
    out.push({
      figmaNode: el.getAttribute('data-figma-node') || '',
      tag: el.tagName.toLowerCase(),
      text: text.trim().slice(0, 80),
      styles: styles,
      box: { x: r.x, y: r.y, width: r.width, height: r.height }
    });
    if (out.length >= 3000) break;
  }
  return out;
})()`

// Extract renders url at the given viewport width (so px line up with the Figma
// frame) and returns the computed styles of every data-figma-node element.
func Extract(ctx context.Context, url string, width int) ([]DOMElement, error) {
	if width <= 0 {
		width = 1280
	}

	tctx, cancel := tab(ctx, 30*time.Second)
	defer cancel()

	var els []DOMElement
	err := chromedp.Run(tctx,
		chromedp.EmulateViewport(int64(width), 900),
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(extractJS, &els),
	)
	if err != nil {
		return nil, fmt.Errorf("DOM extract from %s: %w", url, err)
	}
	return els, nil
}

// Screenshot renders url at the given width and returns a full-page PNG. Used
// for the Tier-2 semantic check and the no-DOM (image) path.
func Screenshot(ctx context.Context, url string, width int) ([]byte, error) {
	if width <= 0 {
		width = 1280
	}
	tctx, cancel := tab(ctx, 30*time.Second)
	defer cancel()

	var buf []byte
	err := chromedp.Run(tctx,
		chromedp.EmulateViewport(int64(width), 900),
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.CaptureScreenshot(&buf),
	)
	if err != nil {
		return nil, fmt.Errorf("screenshot %s: %w", url, err)
	}
	return buf, nil
}

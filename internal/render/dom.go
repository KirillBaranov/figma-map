// Package render drives headless Chrome to render the agent's output and read
// the real, computed values from the DOM. These exact "is" values are the
// deterministic counterpart to the Figma "should-be" tokens, so reconcile
// compares numbers instead of guessing from pixels.
package render

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// waitFontsReady blocks until every @font-face/web-font load the page kicked
// off has actually finished, via the document.fonts.ready promise — replaces
// a fixed sleep, which either races a slow font load (capturing text in its
// fallback-font fallback, the wrong metrics) or wastes time once fonts are
// already loaded. document.fonts.ready resolves even for documents that
// never load any custom font at all, so this is a safe no-op there.
var waitFontsReady = chromedp.ActionFunc(func(ctx context.Context) error {
	var ok bool
	return chromedp.Evaluate(`document.fonts.ready.then(() => true)`, &ok,
		func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}).Do(ctx)
})

// One headless browser is shared across all renders; each call opens a fresh
// tab. This keeps an agent's reconcile loop from launching Chrome on every call.
// The browser is recreated if it dies (e.g. crash in a long-lived MCP server).
var (
	browserMu  sync.Mutex
	browserCtx context.Context
)

// ensureBrowser returns a live shared browser context, (re)creating it if absent
// or dead. Caller need not hold the lock. CHROME_PATH overrides the binary (used
// in CI to point at the action-installed Chrome).
func ensureBrowser() context.Context {
	browserMu.Lock()
	defer browserMu.Unlock()
	if browserCtx == nil || browserCtx.Err() != nil {
		opts := chromedp.DefaultExecAllocatorOptions[:]
		if path := os.Getenv("CHROME_PATH"); path != "" {
			opts = append(opts, chromedp.ExecPath(path))
		}
		// CI containers lack a usable Chrome sandbox; disable it there only
		// (local runs keep the sandbox, rendering untrusted URLs more safely).
		if os.Getenv("CI") != "" {
			opts = append(opts, chromedp.NoSandbox)
		}
		alloc, _ := chromedp.NewExecAllocator(context.Background(), opts...)
		browserCtx, _ = chromedp.NewContext(alloc)
	}
	return browserCtx
}

// resetBrowser discards the shared browser so the next render recreates it.
func resetBrowser() {
	browserMu.Lock()
	defer browserMu.Unlock()
	browserCtx = nil
}

// tab opens a new tab on the shared browser, bounded by a timeout and cancelled
// if the caller's context is done.
func tab(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	tabCtx, cancelTab := chromedp.NewContext(ensureBrowser())
	stop := context.AfterFunc(ctx, cancelTab)
	timed, cancelTimed := context.WithTimeout(tabCtx, timeout)
	return timed, func() { stop(); cancelTimed(); cancelTab() }
}

// withTab runs actions in a fresh tab; if the shared browser had died, it
// recreates it and retries once. The caller's context still bounds the work.
func withTab(ctx context.Context, timeout time.Duration, run func(context.Context) error) error {
	for attempt := 0; attempt < 2; attempt++ {
		tabCtx, cancel := tab(ctx, timeout)
		err := run(tabCtx)
		cancel()
		if err == nil {
			return nil
		}
		// Retry once only if the caller didn't cancel and the browser itself died.
		if attempt == 0 && ctx.Err() == nil && browserCtx != nil && browserCtx.Err() != nil {
			resetBrowser()
			continue
		}
		return err
	}
	return nil
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
    'padding-bottom','padding-left','gap','column-gap','row-gap','opacity',
    'transform','box-shadow'];
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

	var els []DOMElement
	err := withTab(ctx, 30*time.Second, func(tctx context.Context) error {
		return chromedp.Run(tctx,
			chromedp.EmulateViewport(int64(width), 900),
			chromedp.Navigate(url),
			chromedp.WaitReady("body", chromedp.ByQuery),
			waitFontsReady,
			chromedp.Evaluate(extractJS, &els),
		)
	})
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
	var buf []byte
	err := withTab(ctx, 30*time.Second, func(tctx context.Context) error {
		return chromedp.Run(tctx,
			chromedp.EmulateViewport(int64(width), 900),
			chromedp.Navigate(url),
			chromedp.WaitReady("body", chromedp.ByQuery),
			waitFontsReady,
			chromedp.CaptureScreenshot(&buf),
		)
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot %s: %w", url, err)
	}
	return buf, nil
}

// screenshotViewport renders url in a viewport of exactly w×h CSS pixels at
// the given deviceScaleFactor (1 = @1x, 2 = @2x). The returned PNG has
// physical dimensions w*scale × h*scale. Use scale=1 and match against a
// scale=1 Figma export so both images are the same resolution before diffing.
func screenshotViewport(url string, w, h int, scale float64) ([]byte, error) {
	if w <= 0 {
		w = 1280
	}
	if h <= 0 {
		h = 900
	}
	if scale <= 0 {
		scale = 1
	}
	var buf []byte
	err := withTab(ensureBrowser(), 30*time.Second, func(tctx context.Context) error {
		return chromedp.Run(tctx,
			chromedp.EmulateViewport(int64(w), int64(h), chromedp.EmulateScale(scale)),
			chromedp.Navigate(url),
			chromedp.WaitReady("body", chromedp.ByQuery),
			waitFontsReady,
			chromedp.CaptureScreenshot(&buf),
		)
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot %s: %w", url, err)
	}
	return buf, nil
}

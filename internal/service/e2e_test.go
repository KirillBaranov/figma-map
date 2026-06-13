package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/render"
)

// chromeAvailable reports whether headless Chrome can launch (skip e2e if not).
func chromeAvailable(t *testing.T) bool {
	t.Helper()
	_, err := render.Screenshot(context.Background(), "about:blank", 100)
	return err == nil
}

const e2eHTML = `<!doctype html><html><body style="margin:0">
<div data-figma-node="1"
     style="width:200px;height:100px;background:#ffffff;padding:16px;box-sizing:border-box">
  <p data-figma-node="2" style="font-size:24px;font-weight:700;color:#18181b;margin:0">Hi</p>
</div>
</body></html>`

func e2eFrame(textWeight float64) *figma.Node {
	return &figma.Node{
		ID: "1", Type: "FRAME", Bounds: figma.Bounds{Width: 200, Height: 100},
		Styles: &figma.Style{
			Fills:   []figma.Paint{{Type: "SOLID", Color: "#ffffff"}},
			Padding: &figma.Padding{Top: 16, Right: 16, Bottom: 16, Left: 16},
		},
		Children: []figma.Node{{
			ID: "2", Type: "TEXT", Characters: "Hi",
			Bounds: figma.Bounds{X: 16, Y: 16, Width: 168, Height: 28},
			Styles: &figma.Style{
				Fills:      []figma.Paint{{Type: "SOLID", Color: "#18181b"}},
				FontSize:   figma.MaybeNum{Value: 24, Set: true},
				FontWeight: figma.MaybeNum{Value: textWeight, Set: true},
			},
		}},
	}
}

// TestE2E_RenderAlignDiff exercises the real render → align → diff path against a
// live headless Chrome and a local server. Skipped where Chrome is unavailable;
// CI installs Chrome so this guards the render/align layers.
func TestE2E_RenderAlignDiff(t *testing.T) {
	if !chromeAvailable(t) {
		t.Skip("headless Chrome unavailable")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(e2eHTML))
	}))
	defer srv.Close()

	els, err := render.Extract(context.Background(), srv.URL, 200)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	// Matching design → no diffs.
	want := map[string]figmaTarget{}
	collectTargets(e2eFrame(700), 0, 0, true, want)
	got, _ := alignElements(want, els)
	if byEl, _ := tier1Diff(want, got); len(byEl) != 0 {
		t.Errorf("faithful render should match, got %+v", byEl)
	}

	// Design wants font-weight 400 → a real, exact diff is reported.
	want2 := map[string]figmaTarget{}
	collectTargets(e2eFrame(400), 0, 0, true, want2)
	got2, _ := alignElements(want2, els)
	byEl, _ := tier1Diff(want2, got2)
	if len(byEl) != 1 || byEl[0].NodeID != "2" || byEl[0].Diffs[0].Prop != "font-weight" {
		t.Errorf("expected font-weight diff on node 2, got %+v", byEl)
	}
}

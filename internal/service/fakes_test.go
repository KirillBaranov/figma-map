package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/llm"
	"github.com/kirillbaranov/figma-map/internal/storybook"
)

// fakeSource is an in-memory figma.Source for offline orchestration tests.
type fakeSource struct {
	files []figma.File
	doc   *figma.Node
	nodes map[string]*figma.Node
	png   []byte
}

func (f *fakeSource) Ping() error                  { return nil }
func (f *fakeSource) Files() ([]figma.File, error) { return f.files, nil }
func (f *fakeSource) Document(string) (*figma.Node, error) {
	if f.doc == nil {
		return nil, fmt.Errorf("no document")
	}
	return f.doc, nil
}
func (f *fakeSource) Node(_ string, id string) (*figma.Node, error) {
	if n, ok := f.nodes[id]; ok {
		return n, nil
	}
	return nil, fmt.Errorf("no node %s", id)
}
func (f *fakeSource) Selection(string) ([]figma.Node, error) { return nil, nil }
func (f *fakeSource) Screenshot(string, string, figma.ScreenshotOpts) ([]byte, error) {
	return f.png, nil
}

// mockModel is a llm.VisionModel that replays canned JSON keyed by schema name.
type mockModel struct {
	responses map[string]string
	calls     int
	byName    map[string]int
}

func (m *mockModel) VisionJSON(_ context.Context, _ string, _ []llm.Image, name string, out any) error {
	m.calls++
	if m.byName == nil {
		m.byName = map[string]int{}
	}
	m.byName[name]++
	js, ok := m.responses[name]
	if !ok {
		return fmt.Errorf("mock: no response for %q", name)
	}
	return json.Unmarshal([]byte(js), out)
}

// tinyPNG returns a valid 1x1 PNG (enough for catalog/screenshot bytes).
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// tempCatalogBinding writes a one-component catalog + binding to disk and returns
// their paths. The component is Button with a variant prop.
func tempCatalogBinding(t *testing.T) (catalogDir, bindingPath string) {
	t.Helper()
	catalogDir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(catalogDir, "png"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, "png", "ui-button--primary.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := storybook.Catalog{Storybook: "http://x", Stories: []storybook.Story{{
		ID: "ui-button--primary", Component: "Button", Variant: "Primary",
		ImportSymbol: "Button", ImportFrom: "@/components/ui/button",
		PNGPath: filepath.Join("png", "ui-button--primary.png"),
	}}}
	if err := cat.Save(catalogDir); err != nil {
		t.Fatal(err)
	}

	b := binding.Binding{Components: map[string]binding.Component{
		"Button": {
			Symbol: "Button", Import: "@/components/ui/button",
			Props: map[string]binding.Prop{"variant": {Values: []string{"default", "secondary"}}},
		},
	}}
	bindingPath = filepath.Join(t.TempDir(), "binding.yaml")
	if err := b.Save(bindingPath); err != nil {
		t.Fatal(err)
	}
	return catalogDir, bindingPath
}

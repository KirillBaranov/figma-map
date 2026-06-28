package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/figma"
)

// TestMapOrchestration exercises the full Map flow offline: fake Figma source +
// mock vision model + on-disk catalog/binding → JSX.
func TestMapOrchestration(t *testing.T) {
	catalogDir, bindingPath := tempCatalogBinding(t)

	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		png:   tinyPNG(t),
		nodes: map[string]*figma.Node{
			"1:1": {ID: "1:1", Type: "INSTANCE", Name: "button", Children: []figma.Node{
				{Type: "TEXT", Characters: "Continue"},
			}},
		},
	}
	mock := &mockModel{responses: map[string]string{
		"component_match": `{"matches":[{"id":"ui-button--primary","score":1.0,"reason":"x"}],"best_id":"ui-button--primary","confidence":"high","notes":""}`,
		"prop_values":     `{"values":[{"prop":"variant","value":"secondary"}]}`,
	}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	res, err := s.Map(context.Background(), "", bindingPath, catalogDir, "1:1")
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if res.Component != "Button" {
		t.Errorf("component = %q, want Button", res.Component)
	}
	if !strings.Contains(res.JSX, `import { Button } from "@/components/ui/button"`) {
		t.Errorf("missing import in JSX:\n%s", res.JSX)
	}
	if !strings.Contains(res.JSX, `variant="secondary"`) {
		t.Errorf("missing inferred prop in JSX:\n%s", res.JSX)
	}
	if !strings.Contains(res.JSX, ">Continue<") {
		t.Errorf("missing text in JSX:\n%s", res.JSX)
	}
}

// TestPlanDedup verifies Plan maps every instance but pays the LLM once per
// distinct instance (dedup by name+size).
func TestPlanDedup(t *testing.T) {
	catalogDir, bindingPath := tempCatalogBinding(t)

	// Layer names deliberately don't match the catalog's "Button" component
	// by name (e.g. not literally "button") — this test is about the vision
	// dedup-by-cache path, which the tier-1 exact-name match (see
	// TestMatchBoundByName) would otherwise short-circuit before vision ever
	// runs.
	inst := func(id, name string, w, h float64) figma.Node {
		return figma.Node{ID: id, Type: "INSTANCE", Name: name,
			Bounds: figma.Bounds{Width: w, Height: h}}
	}
	frame := &figma.Node{ID: "F", Type: "FRAME", Name: "Page",
		Bounds: figma.Bounds{Width: 1440, Height: 1024},
		Children: []figma.Node{
			inst("1", "cta-1", 100, 40),
			inst("2", "cta-1", 100, 40),       // identical to #1 → dedup
			inst("3", "promo-card", 200, 100), // distinct
		}}

	fake := &fakeSource{
		files: []figma.File{{FileKey: "k"}},
		png:   tinyPNG(t),
		nodes: map[string]*figma.Node{"F": frame},
	}
	mock := &mockModel{responses: map[string]string{
		"component_match": `{"matches":[{"id":"ui-button--primary","score":0.9,"reason":"x"}],"best_id":"ui-button--primary","confidence":"high","notes":""}`,
		"prop_values":     `{"values":[]}`,
	}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	plan, err := s.Plan(context.Background(), "", "F", 0, bindingPath, catalogDir)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Components) != 3 {
		t.Errorf("want 3 mapped components, got %d", len(plan.Components))
	}
	if len(plan.Unmapped) != 0 {
		t.Errorf("want 0 unmapped, got %d", len(plan.Unmapped))
	}
	// Dedup: 2 distinct instances → matched twice, not three times.
	if got := mock.byName["component_match"]; got != 2 {
		t.Errorf("component_match calls = %d, want 2 (dedup of identical instances)", got)
	}
}

// TestBindByName verifies Bind's tier-1 name match: a top-level section
// whose name exactly matches a catalog component skips both the screenshot
// capture and the vision model entirely.
func TestBindByName(t *testing.T) {
	catalogDir, _ := tempCatalogBinding(t)

	doc := &figma.Node{ID: "0:1", Type: "CANVAS", Children: []figma.Node{
		{ID: "1:1", Type: "FRAME", Name: "Button"},
	}}
	fake := &fakeSource{
		files: []figma.File{{FileKey: "k"}},
		doc:   doc,
		png:   tinyPNG(t),
	}
	// No "component_match" response registered: the test fails loudly if
	// vision is consulted for matching. "prop_schema" is still needed —
	// inferring a bound component's prop *schema* from its Storybook variant
	// names is unrelated to this section's Figma-side match and always runs.
	mock := &mockModel{responses: map[string]string{
		"prop_schema": `{"props":[{"name":"variant","values":["default","secondary"]}]}`,
	}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	out := filepath.Join(t.TempDir(), "binding.yaml")
	res, err := s.Bind(context.Background(), "", catalogDir, out)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if len(res.Components) != 1 || res.Components[0] != "Button" {
		t.Errorf("components = %v, want [Button]", res.Components)
	}
	if fake.screenshotCalls != 0 {
		t.Errorf("screenshot called %d time(s), want 0 (matched by name)", fake.screenshotCalls)
	}
	if got := mock.byName["component_match"]; got != 0 {
		t.Errorf("component_match calls = %d, want 0", got)
	}
}

// TestMatchBoundByName verifies the tier-1 deterministic name match: an
// instance whose main-component name (or, failing that, its own layer name)
// exactly matches a catalog component skips the vision model entirely.
func TestMatchBoundByName(t *testing.T) {
	catalogDir, bindingPath := tempCatalogBinding(t)

	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		png:   tinyPNG(t),
		nodes: map[string]*figma.Node{
			// The layer name ("Save CTA") deliberately doesn't match any
			// catalog component — only the main-component lookup (ground
			// truth, fetched separately, see MainComponentName) resolves it.
			"1:1": {ID: "1:1", Type: "INSTANCE", Name: "Save CTA"},
		},
		mainComponentName: map[string]string{"1:1": "Button"},
	}
	// No "component_match" response registered: the test fails loudly (via
	// mockModel's "no response for ..." error) if vision is consulted at all.
	mock := &mockModel{responses: map[string]string{
		"prop_values": `{"values":[]}`,
	}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	res, err := s.Map(context.Background(), "", bindingPath, catalogDir, "1:1")
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if res.Component != "Button" {
		t.Errorf("component = %q, want Button (matched by name, no vision call)", res.Component)
	}
	if res.Score != 1.0 {
		t.Errorf("score = %v, want 1.0 for a deterministic name match", res.Score)
	}
}

// TestMapFullyDeterministic verifies that when both the component identity
// (main-component name) AND its prop values (componentProps) are already
// known from Figma, Map never touches the vision model at all — for either
// "component_match" or "prop_values". The mock has neither response
// registered, so any LLM call fails the test loudly.
func TestMapFullyDeterministic(t *testing.T) {
	catalogDir, bindingPath := tempCatalogBinding(t)

	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		png:   tinyPNG(t),
		nodes: map[string]*figma.Node{
			"1:1": {
				ID: "1:1", Type: "INSTANCE", Name: "Save CTA",
				// Figma's own componentProps key carries the icon glyph +
				// "#nodeId:propId" suffix real VARIANT-vs-TEXT/BOOLEAN
				// properties get — normalization must see past both to land
				// on "variant".
				ComponentProps: map[string]any{"✏️Variant#31450:0": "Secondary"},
			},
		},
		mainComponentName: map[string]string{"1:1": "Button"},
	}
	mock := &mockModel{responses: map[string]string{}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	res, err := s.Map(context.Background(), "", bindingPath, catalogDir, "1:1")
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if res.Component != "Button" {
		t.Errorf("component = %q, want Button", res.Component)
	}
	if got := res.Props["variant"]; got != "secondary" {
		t.Errorf(`props["variant"] = %q, want "secondary" (resolved from Figma componentProps, no vision)`, got)
	}
	if !strings.Contains(res.JSX, `variant="secondary"`) {
		t.Errorf("missing deterministically-resolved prop in JSX:\n%s", res.JSX)
	}
	if mock.calls != 0 {
		t.Errorf("vision model called %d time(s), want 0", mock.calls)
	}
}

// TestMapPropFigmaNameOverride verifies the binding's explicit
// FigmaProperty/ValueMap override: when the code prop name ("variant")
// doesn't match the Figma property name ("Style") and the values use a
// different vocabulary ("M" vs "md"), the override resolves both
// deterministically — no vision call needed.
func TestMapPropFigmaNameOverride(t *testing.T) {
	catalogDir, bindingPath := tempCatalogBinding(t)

	b, err := binding.Load(bindingPath)
	if err != nil {
		t.Fatalf("load binding: %v", err)
	}
	comp := b.Components["Button"]
	comp.Props = map[string]binding.Prop{
		"variant": {
			Values:        []string{"default", "secondary"},
			FigmaProperty: "Style",
			ValueMap:      map[string]string{"Secondary": "secondary"},
		},
		"size": {
			Values:        []string{"md", "lg", "sm"},
			FigmaProperty: "Size",
			ValueMap:      map[string]string{"S": "sm", "M": "md", "L": "lg"},
		},
	}
	b.Components["Button"] = comp
	if err := b.Save(bindingPath); err != nil {
		t.Fatalf("save binding: %v", err)
	}

	fake := &fakeSource{
		files: []figma.File{{FileKey: "k", FileName: "F"}},
		png:   tinyPNG(t),
		nodes: map[string]*figma.Node{
			"1:1": {
				ID: "1:1", Type: "INSTANCE", Name: "Button",
				ComponentProps: map[string]any{"Style": "Secondary", "Size": "S"},
			},
		},
	}
	mock := &mockModel{responses: map[string]string{}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	res, err := s.Map(context.Background(), "", bindingPath, catalogDir, "1:1")
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if got := res.Props["variant"]; got != "secondary" {
		t.Errorf(`props["variant"] = %q, want "secondary"`, got)
	}
	if got := res.Props["size"]; got != "sm" {
		t.Errorf(`props["size"] = %q, want "sm"`, got)
	}
	if mock.calls != 0 {
		t.Errorf("vision model called %d time(s), want 0", mock.calls)
	}
}

// TestPlanUnmapped: when the model finds no match, the instance is reported
// unmapped (never dropped).
func TestPlanUnmapped(t *testing.T) {
	catalogDir, bindingPath := tempCatalogBinding(t)
	frame := &figma.Node{ID: "F", Type: "FRAME", Name: "Page",
		Children: []figma.Node{{ID: "x", Type: "INSTANCE", Name: "mystery",
			Bounds: figma.Bounds{Width: 50, Height: 50}}}}
	fake := &fakeSource{files: []figma.File{{FileKey: "k"}}, png: tinyPNG(t),
		nodes: map[string]*figma.Node{"F": frame}}
	mock := &mockModel{responses: map[string]string{
		"component_match": `{"matches":[],"best_id":"","confidence":"low","notes":""}`,
	}}
	s := &Service{cfg: config.Config{}, src: fake, llm: mock}

	plan, err := s.Plan(context.Background(), "", "F", 0, bindingPath, catalogDir)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Components) != 0 || len(plan.Unmapped) != 1 {
		t.Errorf("want 0 mapped / 1 unmapped, got %d / %d", len(plan.Components), len(plan.Unmapped))
	}
}

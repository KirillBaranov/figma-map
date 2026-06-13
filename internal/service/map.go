package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/codegen"
	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/llm"
	"github.com/kirillbaranov/figma-map/internal/matcher"
	"github.com/kirillbaranov/figma-map/internal/storybook"
)

// MapResult is the outcome of mapping a single Figma node to code.
type MapResult struct {
	NodeID    string            `json:"nodeId"`
	Component string            `json:"component"`
	Score     float64           `json:"score"`
	Props     map[string]string `json:"props,omitempty"`
	Children  string            `json:"children,omitempty"`
	JSX       string            `json:"jsx"`
}

// Map identifies which bound component a Figma node is and generates JSX. Uses
// the LLM (component identity + prop-value inference).
func (s *Service) Map(ctx context.Context, fileKey, bindingPath, catalogDir, nodeID string) (MapResult, error) {
	b, err := binding.Load(bindingPath)
	if err != nil {
		return MapResult{}, fmt.Errorf("load binding (run `bind` first): %w", err)
	}
	catalog, err := storybook.LoadCatalog(catalogDir)
	if err != nil {
		return MapResult{}, fmt.Errorf("load catalog: %w", err)
	}
	client, err := s.llmClient()
	if err != nil {
		return MapResult{}, err
	}
	key, err := s.resolveFileKey(fileKey)
	if err != nil {
		return MapResult{}, err
	}

	png, err := s.src.Screenshot(key, nodeID, figma.ScreenshotOpts{Scale: 2})
	if err != nil {
		return MapResult{}, err
	}
	node, err := s.src.Node(key, nodeID)
	if err != nil {
		return MapResult{}, err
	}

	comp, name, score, err := matchBound(ctx, client, b, catalog, catalogDir, node, png)
	if err != nil {
		return MapResult{}, err
	}

	label := node.FirstText()
	props, err := inferPropValues(ctx, client, png, comp)
	if err != nil {
		// Non-fatal: fall back to defaults.
		props = nil
	}

	jsx := codegen.JSX(codegen.Element{Component: comp, Props: props, Children: label})
	return MapResult{
		NodeID: nodeID, Component: name, Score: score,
		Props: props, Children: label, JSX: jsx,
	}, nil
}

// matchBound matches a rendered node against the binding's components and returns
// the resolved component. Shared by Map and Plan.
func matchBound(ctx context.Context, client llm.VisionModel, b binding.Binding, catalog storybook.Catalog, catalogDir string, node *figma.Node, png []byte) (binding.Component, string, float64, error) {
	candidates, err := boundRepresentatives(b, catalog, catalogDir)
	if err != nil {
		return binding.Component{}, "", 0, err
	}
	res, err := matcher.NewVision(client).Match(ctx, matcher.Target{
		Name:  node.Name,
		Label: node.FirstText(),
		PNG:   png,
	}, candidates)
	if err != nil {
		return binding.Component{}, "", 0, err
	}
	if res.Best == nil {
		return binding.Component{}, "", 0, fmt.Errorf("no bound component matches node %s", node.ID)
	}
	name := res.Best.Story.Component
	comp, ok := b.Components[name]
	if !ok {
		return binding.Component{}, "", 0, fmt.Errorf("matched %q but it is not in the binding", name)
	}
	return comp, name, res.Best.Score, nil
}

// boundRepresentatives returns one catalog screenshot per bound component.
func boundRepresentatives(b binding.Binding, catalog storybook.Catalog, dir string) ([]matcher.CatalogItem, error) {
	seen := map[string]bool{}
	var items []matcher.CatalogItem
	for _, st := range catalog.Stories {
		if _, bound := b.Components[st.Component]; !bound {
			continue
		}
		if seen[st.Component] {
			continue
		}
		seen[st.Component] = true
		png, err := st.PNG(dir)
		if err != nil {
			return nil, err
		}
		items = append(items, matcher.CatalogItem{Story: st, PNG: png})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no catalog screenshots for bound components")
	}
	return items, nil
}

const propValuePromptTmpl = `You are reading prop values from a UI component image.

Component: %q
Available props and allowed values (first is the default):
%s

For each prop, choose the most likely value from its allowed values. If a value
is not clearly distinguishable, use the default. Return one entry per prop.`

// propValues is the structured-output shape (array, not map, for strict schema).
type propValues struct {
	Values []struct {
		Prop  string `json:"prop"`
		Value string `json:"value"`
	} `json:"values"`
}

// inferPropValues reads prop values off the instance image, constrained to the
// binding schema. Values outside the allowed set or equal to the default are
// dropped (defaults are implied).
func inferPropValues(ctx context.Context, model llm.VisionModel, png []byte, comp binding.Component) (map[string]string, error) {
	if len(comp.Props) == 0 {
		return nil, nil
	}
	var schema strings.Builder
	for name, p := range comp.Props {
		fmt.Fprintf(&schema, "- %s: %s\n", name, strings.Join(p.Values, ", "))
	}
	var pv propValues
	if err := model.VisionJSON(ctx, fmt.Sprintf(propValuePromptTmpl, comp.Symbol, schema.String()),
		[]llm.Image{{PNG: png}}, "prop_values", &pv); err != nil {
		return nil, err
	}

	chosen := map[string]string{}
	for _, e := range pv.Values {
		chosen[e.Prop] = e.Value
	}
	out := map[string]string{}
	for name, p := range comp.Props {
		v, ok := chosen[name]
		if !ok || v == "" || v == p.Default() {
			continue
		}
		if allowed(p.Values, v) {
			out[name] = v
		}
	}
	return out, nil
}

func allowed(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

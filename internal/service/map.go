package service

import (
	"context"
	"fmt"
	"sort"
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
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return MapResult{}, err
	}

	png, err := s.src.Screenshot(ctx, key, nodeID, figma.ScreenshotOpts{Scale: 2})
	if err != nil {
		return MapResult{}, err
	}
	node, err := s.src.Node(ctx, key, nodeID)
	if err != nil {
		return MapResult{}, err
	}

	comp, name, score, err := matchBound(ctx, s.src, key, client, b, catalog, catalogDir, node, png)
	if err != nil {
		return MapResult{}, err
	}

	label := node.FirstText()
	props, err := inferPropValues(ctx, client, png, comp, node.ComponentProps)
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
func matchBound(ctx context.Context, src figma.Source, key string, client llm.VisionModel, b binding.Binding, catalog storybook.Catalog, catalogDir string, node *figma.Node, png []byte) (binding.Component, string, float64, error) {
	candidates, err := boundRepresentatives(b, catalog, catalogDir)
	if err != nil {
		return binding.Component{}, "", 0, err
	}

	// Tier 1: deterministic name match, free first — the instance's own
	// layer name already resolves most of the time. Only when it doesn't do
	// we pay for one MainComponentName call: ground truth, immune to this
	// particular layer having been renamed, but not worth fetching when the
	// layer name alone already settled it.
	if comp, name, ok := matchByNameTier(b, node.Name, candidates); ok {
		return comp, name, 1.0, nil
	}
	if mainName, err := src.MainComponentName(ctx, key, node.ID); err == nil && mainName != "" {
		if comp, name, ok := matchByNameTier(b, mainName, candidates); ok {
			return comp, name, 1.0, nil
		}
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

// matchByNameTier resolves a deterministic name match (see matcher.MatchByName)
// to its bound component, if any.
func matchByNameTier(b binding.Binding, name string, candidates []matcher.CatalogItem) (binding.Component, string, bool) {
	item, ok := matcher.MatchByName(name, candidates)
	if !ok {
		return binding.Component{}, "", false
	}
	comp, ok := b.Components[item.Story.Component]
	if !ok {
		return binding.Component{}, "", false
	}
	return comp, item.Story.Component, true
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

// inferPropValues resolves each binding prop's value for a matched instance,
// preferring Figma's own ground truth over a vision guess. Figma's
// componentProps already report the exact value the designer set for every
// VARIANT/BOOLEAN/TEXT property on this instance — no need to ask a vision
// model to "read" a value off a screenshot when the real value is already
// in hand. Vision is only consulted for the leftover props that didn't
// resolve deterministically (no matching Figma property, or one whose value
// doesn't map onto this prop's allowed values), and only over that subset —
// never the whole schema once part of it is already known.
func inferPropValues(ctx context.Context, model llm.VisionModel, png []byte, comp binding.Component, componentProps map[string]any) (map[string]string, error) {
	if len(comp.Props) == 0 {
		return nil, nil
	}

	resolved, unresolved := resolvePropsFromFigma(componentProps, comp.Props)
	if len(unresolved) == 0 {
		return resolved, nil
	}

	visionProps := make(map[string]binding.Prop, len(unresolved))
	for _, name := range unresolved {
		visionProps[name] = comp.Props[name]
	}
	if visionResolved, err := inferPropValuesVision(ctx, model, png, comp.Symbol, visionProps); err == nil {
		for k, v := range visionResolved {
			resolved[k] = v
		}
	}
	// A vision failure on the leftover props is non-fatal — keep whatever
	// was resolved deterministically rather than discarding it.
	return resolved, nil
}

// resolvePropsFromFigma deterministically maps an INSTANCE's own
// componentProps onto the binding's code prop schema by normalized
// name/value equality. Figma's property keys for TEXT/BOOLEAN properties
// carry an icon glyph and a "#nodeId:propId" disambiguator suffix (e.g.
// "✏️Label#31450:0") that NormalizeName's letters/digits-only filter would
// otherwise fold into the name — stripPropertyKeySuffix removes the suffix
// first so the comparison is glyph- and id-noise-free.
func resolvePropsFromFigma(componentProps map[string]any, props map[string]binding.Prop) (resolved map[string]string, unresolved []string) {
	resolved = map[string]string{}
	byNormalizedName := make(map[string]any, len(componentProps))
	for key, v := range componentProps {
		byNormalizedName[matcher.NormalizeName(stripPropertyKeySuffix(key))] = v
	}

	for name, p := range props {
		// FigmaProperty overrides which Figma key to read (e.g. code prop
		// "variant" reading Figma's "Style") — falls back to the prop's own
		// name when names already align.
		figKey := name
		if p.FigmaProperty != "" {
			figKey = p.FigmaProperty
		}
		figVal, ok := byNormalizedName[matcher.NormalizeName(figKey)]
		if !ok {
			unresolved = append(unresolved, name)
			continue
		}
		match, ok := matchAllowedValue(fmt.Sprintf("%v", figVal), p.Values, p.ValueMap)
		if !ok {
			unresolved = append(unresolved, name)
			continue
		}
		if match != p.Default() {
			resolved[name] = match
		}
	}
	sort.Strings(unresolved)
	return resolved, unresolved
}

func stripPropertyKeySuffix(key string) string {
	if i := strings.LastIndex(key, "#"); i >= 0 {
		return key[:i]
	}
	return key
}

// matchAllowedValue resolves a raw Figma (or vision-read) value to one of a
// prop's allowed values. valueMap (an explicit Figma-value → code-value
// override from the binding) is tried first when given; any value without
// an entry there still falls back to plain normalized-name matching.
func matchAllowedValue(value string, allowed []string, valueMap map[string]string) (string, bool) {
	if mapped, ok := valueMap[value]; ok {
		for _, a := range allowed {
			if a == mapped {
				return a, true
			}
		}
	}
	norm := matcher.NormalizeName(value)
	for _, a := range allowed {
		if matcher.NormalizeName(a) == norm {
			return a, true
		}
	}
	return "", false
}

// inferPropValuesVision reads prop values off the instance image for exactly
// the given props (typically the subset inferPropValues couldn't resolve
// deterministically). Values outside the allowed set or equal to the
// default are dropped (defaults are implied).
func inferPropValuesVision(ctx context.Context, model llm.VisionModel, png []byte, symbol string, props map[string]binding.Prop) (map[string]string, error) {
	var schema strings.Builder
	for name, p := range props {
		fmt.Fprintf(&schema, "- %s: %s\n", name, strings.Join(p.Values, ", "))
	}
	var pv propValues
	if err := model.VisionJSON(ctx, fmt.Sprintf(propValuePromptTmpl, symbol, schema.String()),
		[]llm.Image{{PNG: png}}, "prop_values", &pv); err != nil {
		return nil, err
	}

	chosen := map[string]string{}
	for _, e := range pv.Values {
		chosen[e.Prop] = e.Value
	}
	out := map[string]string{}
	for name, p := range props {
		v, ok := chosen[name]
		if !ok || v == "" || v == p.Default() {
			continue
		}
		if match, ok := matchAllowedValue(v, p.Values, p.ValueMap); ok {
			out[name] = match
		}
	}
	return out, nil
}

package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/llm"
	"github.com/kirillbaranov/figma-map/internal/matcher"
	"github.com/kirillbaranov/figma-map/internal/storybook"
)

// BindResult summarizes a bind run.
type BindResult struct {
	Out        string   `json:"out"`
	Sections   int      `json:"sections"`
	Components []string `json:"components"`
}

// Bind screenshots each top-level Figma section, matches it against the catalog,
// infers each matched component's prop schema, and writes a reviewable binding.
// Uses the LLM (matching + prop inference).
func (s *Service) Bind(ctx context.Context, fileKey, catalogDir, out string) (BindResult, error) {
	p := progressFrom(ctx)
	catalog, err := storybook.LoadCatalog(catalogDir)
	if err != nil {
		return BindResult{}, fmt.Errorf("load catalog (run `scan` first): %w", err)
	}
	client, err := s.llmClient()
	if err != nil {
		return BindResult{}, err
	}
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return BindResult{}, err
	}

	reps, err := representatives(catalog, catalogDir)
	if err != nil {
		return BindResult{}, err
	}

	// Only TopLevelFrames() (direct FRAME children of the page) is used below
	// — depth 1 avoids fully styling/resolving every section's whole subtree
	// just to discard it.
	doc, err := s.src.DocumentWithDepth(ctx, key, 1)
	if err != nil {
		return BindResult{}, err
	}
	sections := doc.TopLevelFrames()
	p.emit(fmt.Sprintf("Matching %d Figma sections against %d components …", len(sections), len(reps)))

	matcherV := matcher.NewVision(client)
	matched := map[string]matchedComponent{}

	for _, section := range sections {
		// Tier 1: an exact, unambiguous name match against the catalog is
		// deterministic and free — skip the screenshot capture and vision
		// call entirely when the section's own name already settles it.
		if item, ok := matcher.MatchByName(section.Name, reps); ok {
			comp := item.Story.Component
			p.emit(fmt.Sprintf("  ✓ %s → %s (name match)", section.Name, comp))
			if cur, ok := matched[comp]; !ok || 1.0 > cur.score {
				matched[comp] = matchedComponent{story: item.Story, figmaNode: section.ID, score: 1.0, confidence: "high"}
			}
			continue
		}

		png, err := s.src.Screenshot(ctx, key, section.ID, figma.ScreenshotOpts{Scale: 2})
		if err != nil {
			p.emit(fmt.Sprintf("  ! %s: screenshot failed: %v", section.Name, err))
			continue
		}
		res, err := matcherV.Match(ctx, matcher.Target{
			Name:  section.Name,
			Label: section.FirstText(),
			PNG:   png,
		}, reps)
		if err != nil {
			p.emit(fmt.Sprintf("  ! %s: match failed: %v", section.Name, err))
			continue
		}
		if res.Best == nil {
			p.emit(fmt.Sprintf("  – %s → NO MATCH", section.Name))
			continue
		}
		comp := res.Best.Story.Component
		p.emit(fmt.Sprintf("  ✓ %s → %s (%.2f)", section.Name, comp, res.Best.Score))
		if cur, ok := matched[comp]; !ok || res.Best.Score > cur.score {
			matched[comp] = matchedComponent{
				story:      res.Best.Story,
				figmaNode:  section.ID,
				score:      res.Best.Score,
				confidence: res.Confidence,
			}
		}
	}

	if len(matched) == 0 {
		return BindResult{}, fmt.Errorf("no components matched")
	}

	p.emit("Inferring prop schemas …")
	b := binding.Binding{
		Storybook:  catalog.Storybook,
		FigmaFile:  key,
		Components: map[string]binding.Component{},
	}
	for name, mc := range matched {
		props, err := inferProps(ctx, client, name, variantNames(catalog, name))
		if err != nil {
			p.emit(fmt.Sprintf("  ! %s: prop inference failed: %v", name, err))
		}
		b.Components[name] = binding.Component{
			FigmaNode:  mc.figmaNode,
			Import:     mc.story.ImportFrom,
			Symbol:     mc.story.ImportSymbol,
			Props:      props,
			Confidence: mc.confidence,
		}
	}

	if err := b.Save(out); err != nil {
		return BindResult{}, err
	}
	return BindResult{Out: out, Sections: len(sections), Components: b.ComponentNames()}, nil
}

type matchedComponent struct {
	story      storybook.Story
	figmaNode  string
	score      float64
	confidence string
}

// representatives picks one story per component and loads its screenshot.
func representatives(c storybook.Catalog, dir string) ([]matcher.CatalogItem, error) {
	seen := map[string]bool{}
	var items []matcher.CatalogItem
	for _, st := range c.Stories {
		if seen[st.Component] {
			continue
		}
		seen[st.Component] = true
		png, err := st.PNG(dir)
		if err != nil {
			return nil, fmt.Errorf("load screenshot for %s: %w", st.ID, err)
		}
		items = append(items, matcher.CatalogItem{Story: st, PNG: png})
	}
	return items, nil
}

// variantNames returns the sorted distinct variant names for a component.
func variantNames(c storybook.Catalog, component string) []string {
	var names []string
	for _, st := range c.Stories {
		if st.Component == component {
			names = append(names, st.Variant)
		}
	}
	sort.Strings(names)
	return names
}

const inferPromptTmpl = `You are mapping a code component's Storybook story names to its real props.

Component: %q
Story (variant) names: %s

These story names mix one or more prop dimensions (e.g. visual variant and size). Group them into the component's actual props using the conventions of a typical React component library (e.g. shadcn/ui: a "Primary" story usually maps to variant="default", "Large" to size="lg", "Small" to size="sm").

Rules:
- values must be the CODE prop values (as strings), not the story names.
- list the default value first within each prop.
- omit props you cannot infer.`

// inferResult is the structured-output shape (array, not map, for strict schema).
type inferResult struct {
	Props []struct {
		Name   string   `json:"name"`
		Values []string `json:"values"`
	} `json:"props"`
}

// inferProps asks the LLM to turn variant names into a code prop schema.
func inferProps(ctx context.Context, model llm.VisionModel, component string, variants []string) (map[string]binding.Prop, error) {
	prompt := fmt.Sprintf(inferPromptTmpl, component, strings.Join(variants, ", "))
	var ir inferResult
	if err := model.VisionJSON(ctx, prompt, nil, "prop_schema", &ir); err != nil {
		return nil, err
	}
	props := map[string]binding.Prop{}
	for _, pr := range ir.Props {
		if pr.Name == "" || len(pr.Values) == 0 {
			continue
		}
		props[pr.Name] = binding.Prop{Values: pr.Values}
	}
	return props, nil
}

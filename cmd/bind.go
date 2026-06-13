package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/llm"
	"github.com/kirillbaranov/figma-map/internal/matcher"
	"github.com/kirillbaranov/figma-map/internal/storybook"
	"github.com/spf13/cobra"
)

func newBindCmd() *cobra.Command {
	var (
		fileKey    string
		catalogDir string
		out        string
	)

	c := &cobra.Command{
		Use:   "bind",
		Short: "Match Figma component sections to the catalog and write a binding",
		Long: "bind screenshots each top-level Figma component section, matches it " +
			"against the scanned catalog with a vision LLM, infers each matched " +
			"component's prop schema, and writes a reviewable figma-map.binding.yaml.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			catalog, err := storybook.LoadCatalog(catalogDir)
			if err != nil {
				return fmt.Errorf("load catalog (run `scan` first): %w", err)
			}

			llmClient, err := newLLM(cfg)
			if err != nil {
				return err
			}

			bridge := figma.NewBridge(cfg.Bridge)
			key, err := resolveFileKey(fileKey, cfg, bridge)
			if err != nil {
				return err
			}

			// One representative candidate per component for matching.
			reps, err := representatives(catalog, catalogDir)
			if err != nil {
				return err
			}

			doc, err := bridge.Document(key)
			if err != nil {
				return err
			}
			sections := doc.TopLevelFrames()
			fmt.Printf("Matching %d Figma sections against %d components …\n", len(sections), len(reps))

			matcherV := matcher.NewVision(llmClient)
			// best match per component name, keyed to keep the highest score.
			matched := map[string]matchedComponent{}

			for _, section := range sections {
				png, err := bridge.Screenshot(key, section.ID, figma.ScreenshotOpts{Scale: 2})
				if err != nil {
					fmt.Printf("  ! %s: screenshot failed: %v\n", section.Name, err)
					continue
				}
				res, err := matcherV.Match(context.Background(), matcher.Target{
					Name:  section.Name,
					Label: section.FirstText(),
					PNG:   png,
				}, reps)
				if err != nil {
					fmt.Printf("  ! %s: match failed: %v\n", section.Name, err)
					continue
				}
				if res.Best == nil {
					fmt.Printf("  – %s → NO MATCH\n", section.Name)
					continue
				}
				comp := res.Best.Story.Component
				fmt.Printf("  ✓ %s → %s (%.2f)\n", section.Name, comp, res.Best.Score)
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
				return fmt.Errorf("no components matched")
			}

			// Infer prop schema for each matched component from its variant names.
			fmt.Println("Inferring prop schemas …")
			b := binding.Binding{
				Storybook:  catalog.Storybook,
				FigmaFile:  key,
				Components: map[string]binding.Component{},
			}
			for name, mc := range matched {
				variants := variantNames(catalog, name)
				props, err := inferProps(context.Background(), llmClient, name, variants)
				if err != nil {
					fmt.Printf("  ! %s: prop inference failed: %v\n", name, err)
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
				return err
			}
			fmt.Printf("Wrote %s (%d components). Review before use.\n", out, len(b.Components))
			return nil
		},
	}

	c.Flags().StringVar(&fileKey, "file", "", "Figma file key (default: config or sole connected file)")
	c.Flags().StringVar(&catalogDir, "catalog", "catalog", "catalog directory from `scan`")
	c.Flags().StringVar(&out, "out", "figma-map.binding.yaml", "output binding file")
	return c
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
	for _, s := range c.Stories {
		if seen[s.Component] {
			continue
		}
		seen[s.Component] = true
		png, err := s.PNG(dir)
		if err != nil {
			return nil, fmt.Errorf("load screenshot for %s: %w", s.ID, err)
		}
		items = append(items, matcher.CatalogItem{Story: s, PNG: png})
	}
	return items, nil
}

// variantNames returns the sorted distinct variant names for a component.
func variantNames(c storybook.Catalog, component string) []string {
	var names []string
	for _, s := range c.Stories {
		if s.Component == component {
			names = append(names, s.Variant)
		}
	}
	sort.Strings(names)
	return names
}

const inferPromptTmpl = `You are mapping a code component's Storybook story names to its real props.

Component: %q
Story (variant) names: %s

These story names mix one or more prop dimensions (e.g. visual variant and size). Group them into the component's actual props using the conventions of a typical React component library (e.g. shadcn/ui: a "Primary" story usually maps to variant="default", "Large" to size="lg", "Small" to size="sm").

Return JSON only:
{ "props": { "<codePropName>": { "values": ["<codeValue>", ...] } } }

Rules:
- values must be the CODE prop values, not the story names.
- list the default value first.
- omit props you cannot infer.`

type inferResult struct {
	Props map[string]struct {
		// Values may arrive as strings, booleans, or numbers (e.g. a boolean
		// "disabled" prop), so accept any JSON scalar and stringify.
		Values []any `json:"values"`
	} `json:"props"`
}

// inferProps asks the LLM to turn variant names into a code prop schema.
func inferProps(ctx context.Context, client *llm.Client, component string, variants []string) (map[string]binding.Prop, error) {
	prompt := fmt.Sprintf(inferPromptTmpl, component, strings.Join(variants, ", "))
	reply, err := client.Vision(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}
	m := jsonObjRe.FindString(reply)
	if m == "" {
		return nil, fmt.Errorf("no JSON in reply")
	}
	var ir inferResult
	if err := json.Unmarshal([]byte(m), &ir); err != nil {
		return nil, fmt.Errorf("parse infer result: %w", err)
	}
	props := map[string]binding.Prop{}
	for name, p := range ir.Props {
		values := make([]string, 0, len(p.Values))
		for _, v := range p.Values {
			values = append(values, fmt.Sprintf("%v", v))
		}
		props[name] = binding.Prop{Values: values}
	}
	return props, nil
}

var jsonObjRe = regexp.MustCompile(`(?s)\{.*\}`)

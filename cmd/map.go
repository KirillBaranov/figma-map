package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/codegen"
	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/llm"
	"github.com/kirillbaranov/figma-map/internal/matcher"
	"github.com/kirillbaranov/figma-map/internal/storybook"
	"github.com/spf13/cobra"
)

func newMapCmd() *cobra.Command {
	var (
		fileKey     string
		bindingPath string
		catalogDir  string
	)

	c := &cobra.Command{
		Use:   "map <nodeId>",
		Short: "Generate code for a Figma node using a binding",
		Long: "map screenshots a Figma node, identifies which bound component it is, " +
			"infers prop values against that component's schema, and prints JSX.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			nodeID := args[0]

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			b, err := binding.Load(bindingPath)
			if err != nil {
				return fmt.Errorf("load binding (run `bind` first): %w", err)
			}
			catalog, err := storybook.LoadCatalog(catalogDir)
			if err != nil {
				return fmt.Errorf("load catalog: %w", err)
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

			png, err := bridge.Screenshot(key, nodeID, figma.ScreenshotOpts{Scale: 2})
			if err != nil {
				return err
			}
			node, err := bridge.Node(key, nodeID)
			if err != nil {
				return err
			}
			label := node.FirstText()

			// Candidates: representatives for bound components only.
			candidates, err := boundRepresentatives(b, catalog, catalogDir)
			if err != nil {
				return err
			}

			res, err := matcher.NewVision(llmClient).Match(context.Background(), matcher.Target{
				Name:  node.Name,
				Label: label,
				PNG:   png,
			}, candidates)
			if err != nil {
				return err
			}
			if res.Best == nil {
				return fmt.Errorf("no bound component matches node %s", nodeID)
			}

			compName := res.Best.Story.Component
			comp, ok := b.Components[compName]
			if !ok {
				return fmt.Errorf("matched %q but it is not in the binding", compName)
			}

			props, err := inferPropValues(context.Background(), llmClient, png, comp)
			if err != nil {
				fmt.Printf("// prop inference failed (%v); emitting defaults\n", err)
			}

			jsx := codegen.JSX(codegen.Element{
				Component: comp,
				Props:     props,
				Children:  label,
			})
			fmt.Printf("// %s → %s (%.2f)\n%s\n", nodeID, compName, res.Best.Score, jsx)
			return nil
		},
	}

	c.Flags().StringVar(&fileKey, "file", "", "Figma file key (default: config or sole connected file)")
	c.Flags().StringVar(&bindingPath, "binding", "figma-map.binding.yaml", "binding file from `bind`")
	c.Flags().StringVar(&catalogDir, "catalog", "catalog", "catalog directory from `scan`")
	return c
}

// boundRepresentatives returns one catalog screenshot per bound component, used
// to constrain matching to components the binding knows about.
func boundRepresentatives(b binding.Binding, catalog storybook.Catalog, dir string) ([]matcher.CatalogItem, error) {
	seen := map[string]bool{}
	var items []matcher.CatalogItem
	for _, s := range catalog.Stories {
		if _, bound := b.Components[s.Component]; !bound {
			continue
		}
		if seen[s.Component] {
			continue
		}
		seen[s.Component] = true
		png, err := s.PNG(dir)
		if err != nil {
			return nil, err
		}
		items = append(items, matcher.CatalogItem{Story: s, PNG: png})
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

Look at the image and choose the most likely value for each prop. If a prop's value is not clearly distinguishable, use its default.

Return JSON only: { "<propName>": "<chosenValue>", ... }`

// inferPropValues asks the LLM to read prop values off the instance image,
// constrained to the binding's schema. Values outside the allowed set are
// dropped.
func inferPropValues(ctx context.Context, client *llm.Client, png []byte, comp binding.Component) (map[string]string, error) {
	if len(comp.Props) == 0 {
		return nil, nil
	}

	var schema strings.Builder
	for name, p := range comp.Props {
		fmt.Fprintf(&schema, "- %s: %s\n", name, strings.Join(p.Values, ", "))
	}
	prompt := fmt.Sprintf(propValuePromptTmpl, comp.Symbol, schema.String())

	reply, err := client.Vision(ctx, prompt, []llm.Image{{PNG: png}})
	if err != nil {
		return nil, err
	}
	m := jsonObjRe.FindString(reply)
	if m == "" {
		return nil, fmt.Errorf("no JSON in reply")
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(m), &raw); err != nil {
		return nil, fmt.Errorf("parse prop values: %w", err)
	}

	// Keep only allowed, non-default values (defaults are implied).
	out := map[string]string{}
	for name, p := range comp.Props {
		v, ok := raw[name]
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

package service

import (
	"context"
	"fmt"
	"math"

	"github.com/kirillbaranov/figma-map/internal/binding"
	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/storybook"
)

// Plan is a buildable spec for a frame: the layout container, the component
// instances mapped to code, and an honest list of what couldn't be mapped.
type Plan struct {
	Frame      PlanFrame       `json:"frame"`
	Layout     *PlanLayout     `json:"layout,omitempty"`
	Components []PlanComponent `json:"components"`
	Unmapped   []PlanUnmapped  `json:"unmapped,omitempty"`
}

// PlanFrame identifies the frame being planned.
type PlanFrame struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// PlanLayout is the frame's container layout (from auto-layout).
type PlanLayout struct {
	Direction string         `json:"direction,omitempty"` // row | column
	Gap       *float64       `json:"gap,omitempty"`
	Padding   *figma.Padding `json:"padding,omitempty"`
}

// PlanComponent is one component instance mapped to code.
type PlanComponent struct {
	NodeID     string            `json:"nodeId"`
	Component  string            `json:"component"`
	Symbol     string            `json:"symbol"`
	Import     string            `json:"import"`
	Props      map[string]string `json:"props,omitempty"`
	Text       string            `json:"text,omitempty"`
	Bounds     figma.Bounds      `json:"bounds"`
	Tokens     *Tokens           `json:"tokens,omitempty"`
	Confidence float64           `json:"confidence"`
}

// PlanUnmapped is a component instance figma-map could not map — surfaced
// honestly so the agent builds it by hand rather than dropping it.
type PlanUnmapped struct {
	NodeID string       `json:"nodeId"`
	Name   string       `json:"name"`
	Type   string       `json:"type"`
	Bounds figma.Bounds `json:"bounds"`
	Tokens *Tokens      `json:"tokens,omitempty"`
	Reason string       `json:"reason"`
}

// Plan maps every component instance in a frame to code. Uses the LLM for
// component identity and prop inference (deduped: identical instances cost once).
func (s *Service) Plan(ctx context.Context, fileKey, frameID string, depth int, bindingPath, catalogDir string) (Plan, error) {
	p := progressFrom(ctx)
	b, err := binding.Load(bindingPath)
	if err != nil {
		return Plan{}, fmt.Errorf("load binding (run `bind` first): %w", err)
	}
	catalog, err := storybook.LoadCatalog(catalogDir)
	if err != nil {
		return Plan{}, fmt.Errorf("load catalog: %w", err)
	}
	client, err := s.llmClient()
	if err != nil {
		return Plan{}, err
	}
	key, err := s.resolveFileKey(fileKey)
	if err != nil {
		return Plan{}, err
	}

	frame, err := s.bridge.Node(key, frameID)
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{
		Frame:  PlanFrame{ID: frame.ID, Name: frame.Name, Width: frame.Bounds.Width, Height: frame.Bounds.Height},
		Layout: layoutOf(frame.Styles),
	}

	instances := collectInstances(frame, depth)
	p.emit(fmt.Sprintf("Planning %s: %d component instance(s) …", frame.Name, len(instances)))

	// Dedupe identical instances so the LLM is paid once per distinct one.
	type outcome struct {
		matched bool
		comp    binding.Component
		name    string
		score   float64
		props   map[string]string
	}
	cache := map[string]outcome{}

	for _, inst := range instances {
		k := instKey(inst)
		oc, done := cache[k]
		if !done {
			png, err := s.bridge.Screenshot(key, inst.ID, figma.ScreenshotOpts{Scale: 2})
			if err != nil {
				oc = outcome{}
			} else if comp, name, score, err := matchBound(ctx, client, b, catalog, catalogDir, inst, png); err != nil {
				oc = outcome{}
			} else {
				props, _ := inferPropValues(ctx, client, png, comp)
				oc = outcome{matched: true, comp: comp, name: name, score: score, props: props}
			}
			cache[k] = oc
		}

		if oc.matched {
			plan.Components = append(plan.Components, PlanComponent{
				NodeID: inst.ID, Component: oc.name, Symbol: oc.comp.Symbol, Import: oc.comp.Import,
				Props: oc.props, Text: inst.FirstText(), Bounds: inst.Bounds,
				Tokens: tokensFromStyle(inst.Styles), Confidence: oc.score,
			})
		} else {
			plan.Unmapped = append(plan.Unmapped, PlanUnmapped{
				NodeID: inst.ID, Name: inst.Name, Type: inst.Type, Bounds: inst.Bounds,
				Tokens: tokensFromStyle(inst.Styles), Reason: "no catalog match above threshold",
			})
		}
	}
	return plan, nil
}

// collectInstances returns INSTANCE descendants of frame, up to depth (0 =
// unlimited). The frame itself is not included.
func collectInstances(frame *figma.Node, depth int) []*figma.Node {
	var out []*figma.Node
	var walk func(n *figma.Node, cur int)
	walk = func(n *figma.Node, cur int) {
		for i := range n.Children {
			c := &n.Children[i]
			if c.Type == "INSTANCE" {
				out = append(out, c)
				continue // don't descend into a matched instance's internals
			}
			if depth == 0 || cur < depth {
				walk(c, cur+1)
			}
		}
	}
	walk(frame, 0)
	return out
}

// instKey identifies "the same" instance for dedupe: name + rounded size.
func instKey(n *figma.Node) string {
	return fmt.Sprintf("%s|%d×%d", n.Name, int(math.Round(n.Bounds.Width)), int(math.Round(n.Bounds.Height)))
}

func layoutOf(st *figma.Style) *PlanLayout {
	if st == nil || st.AutoLayout == nil {
		return nil
	}
	l := &PlanLayout{Gap: ptr(st.AutoLayout.Gap), Padding: st.Padding}
	switch st.AutoLayout.Direction {
	case "HORIZONTAL":
		l.Direction = "row"
	case "VERTICAL":
		l.Direction = "column"
	}
	return l
}

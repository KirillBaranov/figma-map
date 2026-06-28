// Package binding models the persistent figma-map.binding.yaml artifact that
// maps a Figma component library to a code component library. It is produced
// once by `bind` (AI-assisted), reviewed by a human, then consumed
// deterministically by `map`.
package binding

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// Binding is the whole mapping document.
type Binding struct {
	// Storybook records which catalog the binding was generated against.
	Storybook string `yaml:"storybook,omitempty"`
	// FigmaFile records the source Figma file key.
	FigmaFile string `yaml:"figmaFile,omitempty"`
	// Components maps a code component name to its binding.
	Components map[string]Component `yaml:"components"`
}

// Component binds one Figma section to one code component and describes its
// props so `map` can emit correct JSX.
type Component struct {
	// FigmaNode is the id of the matched Figma section/component.
	FigmaNode string `yaml:"figmaNode"`
	// Import is the module path, e.g. "@/components/ui/button".
	Import string `yaml:"import"`
	// Symbol is the imported JSX symbol, e.g. "Button".
	Symbol string `yaml:"symbol"`
	// Props maps a code prop name to its allowed values. The first value is
	// treated as the default. Used by `map` to constrain prop inference.
	Props map[string]Prop `yaml:"props,omitempty"`
	// Confidence is the matcher's confidence at bind time (advisory).
	Confidence string `yaml:"confidence,omitempty"`
}

// Prop describes a single code prop and its allowed values.
type Prop struct {
	// Values are the allowed values; index 0 is the default.
	Values []string `yaml:"values"`
	// FigmaProperty is the Figma componentProperty key this prop reads its
	// value from (e.g. "Style" for a code prop named "variant"). Optional —
	// omit when the Figma property is already named the same as this prop
	// (case/punctuation-insensitive, e.g. both "size"/"Size").
	FigmaProperty string `yaml:"figmaProperty,omitempty"`
	// ValueMap maps a Figma componentProperty value to its code value (e.g.
	// {"Primary": "primary"}), for when they don't already align after
	// normalization (e.g. Figma "M" vs code "md"). Optional — omit when
	// every value already aligns; a Figma value with no entry here still
	// falls back to plain normalized matching.
	ValueMap map[string]string `yaml:"valueMap,omitempty"`
}

// Load reads a binding document from path.
func Load(path string) (Binding, error) {
	var b Binding
	data, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	if err := yaml.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("parse binding %s: %w", path, err)
	}
	return b, nil
}

// Save writes the binding document to path.
func (b Binding) Save(path string) error {
	data, err := yaml.Marshal(b)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ComponentNames returns the bound component names, sorted for stable output.
func (b Binding) ComponentNames() []string {
	names := make([]string, 0, len(b.Components))
	for n := range b.Components {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Default returns the default value for a prop (its first listed value), or "".
func (p Prop) Default() string {
	if len(p.Values) == 0 {
		return ""
	}
	return p.Values[0]
}

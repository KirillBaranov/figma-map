package service

import (
	"github.com/kirillbaranov/figma-map/internal/binding"
)

// ComponentInfo describes one bound component for `list`.
type ComponentInfo struct {
	Name   string              `json:"name"`
	Import string              `json:"import"`
	Symbol string              `json:"symbol"`
	Props  map[string][]string `json:"props,omitempty"`
}

// ListResult wraps the components. It is an object (not a bare array) because
// MCP tool output schemas must have type "object".
type ListResult struct {
	Components []ComponentInfo `json:"components"`
}

// List returns the components in a binding. Deterministic, no API key needed.
func (s *Service) List(bindingPath string) (ListResult, error) {
	b, err := binding.Load(bindingPath)
	if err != nil {
		return ListResult{}, err
	}
	var out []ComponentInfo
	for _, name := range b.ComponentNames() {
		c := b.Components[name]
		props := map[string][]string{}
		for pn, p := range c.Props {
			props[pn] = p.Values
		}
		out = append(out, ComponentInfo{
			Name:   name,
			Import: c.Import,
			Symbol: c.Symbol,
			Props:  props,
		})
	}
	return ListResult{Components: out}, nil
}

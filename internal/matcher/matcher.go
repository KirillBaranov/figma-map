// Package matcher decides which code component a Figma image corresponds to.
// The Matcher interface lets the vision implementation be swapped later for an
// embedding-based retriever without touching callers.
package matcher

import (
	"context"

	"github.com/kirillbaranov/figma-map/internal/storybook"
)

// Candidate is one scored catalog match.
type Candidate struct {
	Story  storybook.Story
	Score  float64
	Reason string
}

// Result is the outcome of a match. Best is nil when nothing crosses the
// confidence threshold (a genuine NO MATCH).
type Result struct {
	Best       *Candidate
	Confidence string // "high" | "medium" | "low"
	Notes      string
	Candidates []Candidate
}

// Target is the thing being matched: a rendered Figma node plus light context
// signals (its name and any text label) that help disambiguate look-alikes.
type Target struct {
	Name  string
	Label string
	PNG   []byte
}

// CatalogItem pairs a catalog story with its loaded screenshot bytes. Callers
// decide which items to offer (e.g. one representative per component for bind,
// or only bound components for map).
type CatalogItem struct {
	Story storybook.Story
	PNG   []byte
}

// Matcher maps a Target to a code component from the given candidates.
type Matcher interface {
	Match(ctx context.Context, target Target, candidates []CatalogItem) (Result, error)
}

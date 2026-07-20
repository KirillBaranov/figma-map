package service

import "fmt"

// Issue is one entry in the cascade's unified, cross-source issue-list (see
// docs/design/verification-loop-strategy.md) — a single machine-readable
// finding an agent can act on directly, in the same shape regardless of
// which layer of the verification cascade produced it (structured diff,
// attributed pixeldiff, or the VLM tier). Additive alongside the
// longer-standing ByElement/Regions shapes, not a replacement for them.
type Issue struct {
	NodeID string `json:"nodeId"`
	// DOMSelector targets the rendered element directly, when the tool is
	// confident enough in the match to hand out a selector — empty for
	// spatially-aligned (untagged) matches, where handing out a selector
	// would overstate the confidence.
	DOMSelector string  `json:"domSelector,omitempty"`
	Property    string  `json:"property"`
	Expected    string  `json:"expected"`
	Actual      string  `json:"actual"`
	Severity    string  `json:"severity"`   // major | minor | advisory
	Confidence  float64 `json:"confidence"` // 0-1; lower for spatially-aligned matches
	Source      string  `json:"source"`     // structured | pixel | vlm
}

// IssuesFromClusters flattens attributed-pixeldiff Clusters into the unified
// Issue shape. There's no Figma node id at the pixel-diff layer (no
// structured tree was walked to get here) — NodeID is left empty; the
// region's own coordinates are the only address a pixel-level finding has.
func IssuesFromClusters(clusters []Cluster) []Issue {
	var issues []Issue
	for _, c := range clusters {
		region := fmt.Sprintf("region (%d,%d,%dx%d)", c.X, c.Y, c.W, c.H)
		var property, expected, actual string
		confidence := 0.7 // deterministic classification, but pixel-level attribution is inherently approximate
		switch c.Kind {
		case "shift":
			property = "position"
			expected = fmt.Sprintf("%s shifted by (%d,%d)px", region, c.OffsetX, c.OffsetY)
			actual = region
		case "color":
			property = "color"
			expected = fmt.Sprintf("%s: color matches design", region)
			actual = fmt.Sprintf("%s: color differs", region)
		default: // "other" — the catch-all: size, missing/extra element, or unmodeled
			property = "appearance"
			expected = fmt.Sprintf("%s matches design", region)
			actual = fmt.Sprintf("%s differs, cause not classified — inspect visually", region)
			confidence = 0.4
		}
		issues = append(issues, Issue{
			Property:   property,
			Expected:   expected,
			Actual:     actual,
			Severity:   "major",
			Confidence: confidence,
			Source:     "pixel",
		})
	}
	return issues
}

// issuesFromElements flattens tier1Diff's per-element field diffs into the
// unified Issue shape. spatial lists node ids matched by geometry rather
// than a data-figma-node tag (see alignElements) — those get a lower
// confidence and no DOM selector, since the match itself is a guess.
func issuesFromElements(byElement []ElementDiff, spatial []string) []Issue {
	spatialSet := make(map[string]bool, len(spatial))
	for _, id := range spatial {
		spatialSet[id] = true
	}

	var issues []Issue
	for _, e := range byElement {
		confidence := 1.0
		selector := fmt.Sprintf("[data-figma-node=%q]", e.NodeID)
		if spatialSet[e.NodeID] {
			confidence = 0.6
			selector = ""
		}
		for _, d := range e.Diffs {
			severity := "major"
			if d.Advisory {
				severity = "advisory"
			}
			issues = append(issues, Issue{
				NodeID:      e.NodeID,
				DOMSelector: selector,
				Property:    d.Prop,
				Expected:    d.Should,
				Actual:      d.Is,
				Severity:    severity,
				Confidence:  confidence,
				Source:      "structured",
			})
		}
	}
	return issues
}

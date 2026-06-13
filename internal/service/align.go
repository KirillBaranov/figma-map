package service

import (
	"sort"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/render"
)

// alignElements maps each design target to a rendered DOM element. Targets that
// carry a data-figma-node tag are matched exactly; the rest are matched by
// geometry/type/text so reconcile works on existing, untagged implementations.
//
// Returns the matched element per target id and the set of ids that were matched
// spatially (lower confidence — the caller flags them in the report).
func alignElements(want map[string]figmaTarget, els []render.DOMElement) (map[string]render.DOMElement, []string) {
	matched := make(map[string]render.DOMElement)
	used := make(map[int]bool)

	// Pass 1 — exact, by data-figma-node.
	tagIndex := map[string]int{}
	for i, e := range els {
		if e.FigmaNode != "" {
			tagIndex[e.FigmaNode] = i
		}
	}
	for id := range want {
		if i, ok := tagIndex[id]; ok {
			matched[id] = els[i]
			used[i] = true
		}
	}

	// Pass 2 — spatial, for the remaining targets. Both sides are normalized by
	// the same reference (the frame extent): the DOM is rendered at the frame's
	// width, so figma and DOM coordinates share a px space. Using one reference
	// preserves absolute position, so a far-off element does not falsely match.
	ext := figmaExtent(want)
	if ext.w == 0 || ext.h == 0 {
		return matched, nil // nothing to normalize against
	}

	// Largest targets first so big containers claim their element before small
	// children compete for it.
	ids := make([]string, 0, len(want))
	for id := range want {
		if _, done := matched[id]; !done {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(a, b int) bool {
		return area(want[ids[a]].box) > area(want[ids[b]].box)
	})

	var spatial []string
	for _, id := range ids {
		ft := want[id]
		fb := normalize(box{ft.box.X, ft.box.Y, ft.box.Width, ft.box.Height}, ext)

		best, bestScore := -1, alignThreshold
		for i, e := range els {
			if used[i] {
				continue
			}
			db := normalize(box{e.Box.X, e.Box.Y, e.Box.Width, e.Box.Height}, ext)
			score := iou(fb, db)
			if ft.typ == "TEXT" && textOverlap(ft.text, e.Text) {
				score += 0.3
			}
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		if best >= 0 {
			matched[id] = els[best]
			used[best] = true
			spatial = append(spatial, id)
		}
	}
	sort.Strings(spatial)
	return matched, spatial
}

// alignThreshold is the minimum score (normalized IoU, plus any text bonus) for a
// spatial match to be accepted.
const alignThreshold = 0.3

type box struct{ x, y, w, h float64 }
type extent struct{ minX, minY, w, h float64 }

func area(b figma.Bounds) float64 { return b.Width * b.Height }

func figmaExtent(want map[string]figmaTarget) extent {
	var maxX, maxY float64
	for _, t := range want {
		maxX = max(maxX, t.box.X+t.box.Width)
		maxY = max(maxY, t.box.Y+t.box.Height)
	}
	return extent{0, 0, maxX, maxY}
}

// normalize maps a box into [0,1] relative to ext, removing global offset/scale.
func normalize(b box, ext extent) box {
	return box{
		x: (b.x - ext.minX) / ext.w,
		y: (b.y - ext.minY) / ext.h,
		w: b.w / ext.w,
		h: b.h / ext.h,
	}
}

// iou is the intersection-over-union of two normalized boxes.
func iou(a, b box) float64 {
	ix := max(0, min(a.x+a.w, b.x+b.w)-max(a.x, b.x))
	iy := max(0, min(a.y+a.h, b.y+b.h)-max(a.y, b.y))
	inter := ix * iy
	if inter <= 0 {
		return 0
	}
	union := a.w*a.h + b.w*b.h - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

// textOverlap reports whether two text snippets plausibly refer to the same
// content (case-insensitive containment either way).
func textOverlap(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

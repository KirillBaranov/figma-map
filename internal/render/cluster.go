package render

import "image"

// Cluster is one connected component of differing pixels — a real detected
// region (via flood-fill over the diff mask), not an arbitrary fixed-grid
// cell — classified by its most likely cause. This is the "attributed"
// half of attributed pixeldiff: Regions (see DiffResult) says where; Cluster
// says why.
type Cluster struct {
	X, Y, W, H int
	DiffPixels int
	// Kind is "shift" (a translation explains most of this cluster's diff),
	// "color" (aligned, but the average color differs), or "other" (neither
	// — could be a size or missing/extra-element difference, or something
	// this heuristic doesn't model).
	Kind string
	// OffsetX/OffsetY are set when Kind == "shift": the translation in px
	// (added to the implementation's coordinates) that best explains this
	// cluster's diff against the reference.
	OffsetX, OffsetY int
}

// clusterMinPixels drops connected components this small — single stray
// antialiased pixels aren't a defect worth reporting as their own region.
const clusterMinPixels = 6

// shiftSearchRadius bounds the brute-force offset search so a single large
// cluster can't blow up the cost of classification.
const shiftSearchRadius = 16

// shiftSearchMaxArea caps the bounding-box area a cluster can have and still
// get a shift search; larger clusters fall straight to color/other
// classification (the search cost is O(area × searchWindow²)).
const shiftSearchMaxArea = 400 * 400

// clusterAndClassify groups a boolean diff mask into connected components,
// classifies each, and applies induced-diff subtraction: a cluster fully
// explained by the same translation as a larger "shift" cluster nearby is
// dropped rather than reported as its own defect (one real shift shouldn't
// cascade into N noisy regions below it).
func clusterAndClassify(ref, got *image.RGBA, mask []bool, w, h int, colorTol uint8) []Cluster {
	comps := connectedComponents(mask, w, h)

	clusters := make([]Cluster, 0, len(comps))
	for _, c := range comps {
		if c.diffPixels < clusterMinPixels {
			continue
		}
		cl := Cluster{X: c.x0, Y: c.y0, W: c.x1 - c.x0, H: c.y1 - c.y0, DiffPixels: c.diffPixels}
		cl.Kind, cl.OffsetX, cl.OffsetY = classifyCluster(ref, got, cl, w, h, colorTol)
		clusters = append(clusters, cl)
	}
	sortClustersByDiffDesc(clusters)

	return subtractInducedDiffs(ref, got, clusters, w, h, colorTol)
}

// classifyCluster decides why this region differs: shift, color, or other.
func classifyCluster(ref, got *image.RGBA, cl Cluster, w, h int, colorTol uint8) (kind string, dx, dy int) {
	area := cl.W * cl.H
	if area > 0 && area <= shiftSearchMaxArea {
		bestDX, bestDY, bestCount := bestShift(ref, got, cl.X, cl.Y, cl.W, cl.H, w, h, colorTol)
		// The shift must explain at least half of this cluster's diff to
		// count — otherwise it's coincidental, not the actual defect.
		if (bestDX != 0 || bestDY != 0) && bestCount < cl.DiffPixels/2 {
			return "shift", bestDX, bestDY
		}
	}

	rr, rg, rb := avgColor(ref, cl.X, cl.Y, cl.W, cl.H)
	gr, gg, gb := avgColor(got, cl.X, cl.Y, cl.W, cl.H)
	if colorDistance(rr, rg, rb, gr, gg, gb) > float64(colorTol)*2 {
		return "color", 0, 0
	}
	return "other", 0, 0
}

// bestShift brute-force searches offsets within shiftSearchRadius for the
// translation of got (relative to ref) that minimizes the diff-pixel count
// over the cluster's bounding box — a real (if bounded) cross-correlation,
// not a guess.
func bestShift(ref, got *image.RGBA, x0, y0, cw, ch, w, h int, colorTol uint8) (dx, dy, count int) {
	bestCount := diffCountAtOffset(ref, got, x0, y0, cw, ch, w, h, 0, 0, colorTol)
	bestDX, bestDY := 0, 0
	for oy := -shiftSearchRadius; oy <= shiftSearchRadius; oy++ {
		for ox := -shiftSearchRadius; ox <= shiftSearchRadius; ox++ {
			if ox == 0 && oy == 0 {
				continue
			}
			c := diffCountAtOffset(ref, got, x0, y0, cw, ch, w, h, ox, oy, colorTol)
			if c < bestCount {
				bestCount, bestDX, bestDY = c, ox, oy
			}
		}
	}
	return bestDX, bestDY, bestCount
}

// diffCountAtOffset counts how many pixels in ref's [x0,y0,cw,ch) box still
// differ from got when got is sampled at (x+ox, y+oy) instead of (x,y).
// Pixels that land outside got's bounds under the offset are not counted
// (neither matching nor differing) so the search doesn't get pulled toward
// large offsets that shift most of the box off-canvas.
func diffCountAtOffset(ref, got *image.RGBA, x0, y0, cw, ch, w, h, ox, oy int, colorTol uint8) int {
	count := 0
	for y := y0; y < y0+ch; y++ {
		gy := y + oy
		if gy < 0 || gy >= h {
			continue
		}
		for x := x0; x < x0+cw; x++ {
			gx := x + ox
			if gx < 0 || gx >= w {
				continue
			}
			rPx := ref.RGBAAt(x, y)
			gPx := got.RGBAAt(gx, gy)
			if max8(absDiff(rPx.R, gPx.R), absDiff(rPx.G, gPx.G), absDiff(rPx.B, gPx.B)) > colorTol {
				count++
			}
		}
	}
	return count
}

// avgColor returns the mean R/G/B of a box (excluding nothing — bounds are
// assumed already clamped to the image by the caller).
func avgColor(img *image.RGBA, x0, y0, w, h int) (r, g, b float64) {
	n := 0
	var sr, sg, sb float64
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			px := img.RGBAAt(x, y)
			sr += float64(px.R)
			sg += float64(px.G)
			sb += float64(px.B)
			n++
		}
	}
	if n == 0 {
		return 0, 0, 0
	}
	return sr / float64(n), sg / float64(n), sb / float64(n)
}

func colorDistance(r1, g1, b1, r2, g2, b2 float64) float64 {
	dr, dg, db := r1-r2, g1-g2, b1-b2
	return (abs(dr) + abs(dg) + abs(db)) / 3
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// subtractInducedDiffs drops clusters fully explained by the same
// translation as a larger "shift" cluster — the doc's "one shift found
// virtually shifted → re-diffed" step, so a single real defect doesn't
// cascade into N noisy regions below it.
func subtractInducedDiffs(ref, got *image.RGBA, clusters []Cluster, w, h int, colorTol uint8) []Cluster {
	var primary *Cluster
	for i := range clusters {
		if clusters[i].Kind == "shift" {
			primary = &clusters[i]
			break // already sorted by DiffPixels desc — first shift cluster is the primary
		}
	}
	if primary == nil {
		return clusters
	}

	out := make([]Cluster, 0, len(clusters))
	out = append(out, *primary)
	for _, c := range clusters {
		if c.X == primary.X && c.Y == primary.Y && c.W == primary.W && c.H == primary.H {
			continue // the primary cluster itself, already appended
		}
		underPrimary := diffCountAtOffset(ref, got, c.X, c.Y, c.W, c.H, w, h, primary.OffsetX, primary.OffsetY, colorTol)
		if underPrimary < c.DiffPixels/3 {
			continue // absorbed: the primary shift also explains this cluster
		}
		out = append(out, c)
	}
	sortClustersByDiffDesc(out)
	return out
}

func sortClustersByDiffDesc(cs []Cluster) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].DiffPixels > cs[j-1].DiffPixels; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

type component struct {
	x0, y0, x1, y1 int // bounding box, x1/y1 exclusive
	diffPixels     int
}

// connectedComponents groups a boolean diff mask (row-major, w×h) into
// 4-connected regions via iterative BFS (no recursion, so large contiguous
// diffs — e.g. "the whole page is red" — can't blow the stack).
func connectedComponents(mask []bool, w, h int) []component {
	visited := make([]bool, len(mask))
	var comps []component
	queue := make([]int, 0, 256)

	for start := 0; start < len(mask); start++ {
		if !mask[start] || visited[start] {
			continue
		}
		visited[start] = true
		queue = queue[:0]
		queue = append(queue, start)

		sx, sy := start%w, start/w
		c := component{x0: sx, y0: sy, x1: sx + 1, y1: sy + 1}

		for len(queue) > 0 {
			idx := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			x, y := idx%w, idx/w

			if x < c.x0 {
				c.x0 = x
			}
			if x+1 > c.x1 {
				c.x1 = x + 1
			}
			if y < c.y0 {
				c.y0 = y
			}
			if y+1 > c.y1 {
				c.y1 = y + 1
			}
			c.diffPixels++

			neighbors := [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}}
			for _, n := range neighbors {
				nx, ny := n[0], n[1]
				if nx < 0 || nx >= w || ny < 0 || ny >= h {
					continue
				}
				ni := ny*w + nx
				if mask[ni] && !visited[ni] {
					visited[ni] = true
					queue = append(queue, ni)
				}
			}
		}
		comps = append(comps, c)
	}
	return comps
}

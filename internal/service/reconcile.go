package service

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/render"
)

// FieldDiff is one property that differs, with exact is/should values. Advisory
// marks diffs that may be content-driven (e.g. width/height) and so should not
// block a match — the agent triages them rather than blindly "fixing" them.
type FieldDiff struct {
	Prop     string `json:"prop"`
	Is       string `json:"is"`
	Should   string `json:"should"`
	Advisory bool   `json:"advisory,omitempty"`
}

// ElementDiff groups the differing properties of one element.
type ElementDiff struct {
	NodeID string      `json:"nodeId"`
	Name   string      `json:"name"`
	Diffs  []FieldDiff `json:"diffs"`
}

// UnmeasuredNode is a token-bearing design node with no matching DOM element.
// Actionable=true means the agent should tag it (data-figma-node) and build it;
// false means it's decorative/image content that isn't DOM-measurable anyway.
type UnmeasuredNode struct {
	NodeID     string `json:"nodeId"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Actionable bool   `json:"actionable"`
	Reason     string `json:"reason"`
}

// SemanticFinding is a Tier-2 (LLM) observation Tier 1 can't measure.
type SemanticFinding struct {
	Kind     string `json:"kind"`
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
}

// Coverage reports how much of the design was actually verified, so a "match"
// comes with an honest confidence rather than implying full coverage.
type Coverage struct {
	Measured int `json:"measured"`
	Targets  int `json:"targets"`
}

// Diff is the reconcile result: a report the agent can act on or hand to a
// human — deterministic per-element field diffs (Tier 1), honest unmeasured
// nodes, coverage, and optional semantic findings (Tier 2).
type Diff struct {
	Match     bool          `json:"match"`
	Remaining int           `json:"remaining"` // fixable (non-advisory) diffs
	Advisory  int           `json:"advisory"`  // content-driven diffs (don't block match)
	Coverage  Coverage      `json:"coverage"`
	ByElement []ElementDiff `json:"byElement,omitempty"`
	// SpatiallyAligned lists node ids matched to a DOM element by geometry rather
	// than a data-figma-node tag — lower confidence, so the report flags them.
	SpatiallyAligned []string          `json:"spatiallyAligned,omitempty"`
	Unmeasured       []UnmeasuredNode  `json:"unmeasured,omitempty"`
	Semantic         []SemanticFinding `json:"semantic,omitempty"`
	// Issues is ByElement flattened into the cascade's unified, cross-source
	// shape (see issue.go) — additive alongside ByElement, not a replacement;
	// existing CLI/MCP consumers keep working off ByElement unchanged.
	Issues []Issue `json:"issues,omitempty"`
}

// tolerances for deterministic comparison (sub-pixel/font metrics make exact
// equality unattainable; the target is spec-perfect within these bounds).
const (
	tolSize    = 0.5 // px: font-size, radius, border, line-height
	tolSpacing = 1.0 // px: padding, gap
	tolBox     = 2.0 // px: element width/height (layout rounding)
)

type figmaTarget struct {
	tokens *Tokens
	typ    string
	name   string
	text   string
	box    figma.Bounds // absolute within the frame (origin 0,0)
	// layoutPositioning mirrors Figma's own auto-layout escape hatch
	// (figma.Style.LayoutPositioning, "ABSOLUTE" or ""). parentAutoLayout is
	// true when this node's own parent uses auto-layout. Together they say
	// whether a DOM `position: absolute` on this node is legitimate (Figma
	// declared it as an escape hatch) or a structure-lint violation (the
	// design never asked for it) — see the "position" check in compareNode.
	layoutPositioning string
	parentAutoLayout  bool
	// renderBounds is this node's post-effects, post-transform render bounds
	// (Figma's absoluteRenderBounds), converted to frame-relative space —
	// nil unless the caller opted into geoDiff (see Reconcile). Compared
	// against the DOM's post-transform getBoundingClientRect() in
	// compareNode when the element is CSS-transformed, instead of just
	// skipping the box check as today's declared-Bounds comparison must.
	renderBounds *figma.Bounds
}

// Reconcile compares a Figma node against the agent's rendered output. story or
// url drive the deterministic Tier 1 (DOM computed styles vs Figma tokens);
// imagePath falls back to Tier 2 only (no DOM). semantic enables the Tier-2 LLM
// check (requires an API key).
func (s *Service) Reconcile(ctx context.Context, fileKey, nodeID, story, url, imagePath string, semantic, geoDiff bool) (Diff, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return Diff{}, err
	}
	// RenderBounds forces a Figma render per node, so only request it when
	// geoDiff is opted in (see figma.NodeOptions).
	frame, err := s.src.NodeWithOptions(ctx, key, nodeID, figma.NodeOptions{RenderBounds: geoDiff})
	if err != nil {
		return Diff{}, err
	}

	// No-DOM path: only a flat image was provided.
	if story == "" && url == "" {
		if imagePath == "" {
			return Diff{}, fmt.Errorf("provide one of --story, --url, or --image")
		}
		return s.reconcileImage(ctx, key, frame, imagePath)
	}

	renderURL := url
	if story != "" {
		renderURL = fmt.Sprintf("%s/iframe.html?id=%s&viewMode=story", s.cfg.Storybook, story)
	}
	width := int(math.Round(frame.Bounds.Width))

	els, err := render.Extract(ctx, renderURL, width)
	if err != nil {
		return Diff{}, err
	}

	// The frame's own renderBounds origin: absoluteRenderBounds is in
	// absolute page coordinates, but figmaTarget.box (and the DOM) are
	// frame-relative — subtracting this converts each descendant's render
	// bounds into the same frame-relative space, the same way collectTargets
	// already does for the regular declared Bounds.
	var frameRenderOriginX, frameRenderOriginY float64
	if frame.RenderBounds != nil {
		frameRenderOriginX, frameRenderOriginY = frame.RenderBounds.X, frame.RenderBounds.Y
	}

	want := map[string]figmaTarget{}
	collectTargets(frame, 0, 0, true, false, frameRenderOriginX, frameRenderOriginY, want)

	// Align design nodes to DOM elements: exact by data-figma-node where present,
	// otherwise by geometry/type/text (for existing, untagged implementations).
	got, spatial := alignElements(want, els)

	byElement, unmeasured := tier1Diff(want, got)
	remaining, advisory := 0, 0
	for _, e := range byElement {
		for _, d := range e.Diffs {
			if d.Advisory {
				advisory++
			} else {
				remaining++
			}
		}
	}

	diff := Diff{
		Match:            remaining == 0,
		Remaining:        remaining,
		Advisory:         advisory,
		Coverage:         Coverage{Measured: len(want) - len(unmeasured), Targets: len(want)},
		ByElement:        byElement,
		Unmeasured:       unmeasured,
		SpatiallyAligned: spatial,
		Issues:           issuesFromElements(byElement, spatial),
	}

	if semantic {
		crops := semanticCrops(want, unmeasured, spatial)
		findings, err := s.tier2(ctx, key, frame, renderURL, width, crops)
		if err == nil {
			diff.Semantic = findings
			if hasMajor(findings) {
				diff.Match = false
			}
		}
	}
	return diff, nil
}

// semanticCrops builds Tier-2 crop regions from what Tier-1 couldn't fully
// resolve: actionable-unmeasured nodes (may be genuinely missing from the
// render — exactly what the VLM's "missing/extra" check looks for) and
// spatially-aligned matches (lower-confidence — worth a second look).
// Returns nil when everything was cleanly tag-matched, so tier2 falls back
// to its original whole-frame behavior in the common, confident case.
func semanticCrops(want map[string]figmaTarget, unmeasured []UnmeasuredNode, spatial []string) []CropRegion {
	var crops []CropRegion
	seen := map[string]bool{}
	add := func(id string) {
		if seen[id] {
			return
		}
		ft, ok := want[id]
		if !ok || ft.box.Width <= 0 || ft.box.Height <= 0 {
			return
		}
		seen[id] = true
		crops = append(crops, CropRegion{
			X: int(ft.box.X), Y: int(ft.box.Y), W: int(ft.box.Width), H: int(ft.box.Height),
		})
	}
	for _, u := range unmeasured {
		if u.Actionable {
			add(u.NodeID)
		}
	}
	for _, id := range spatial {
		add(id)
	}
	return crops
}

// collectTargets walks the frame and records every node that carries tokens,
// accumulating each node's absolute position within the frame (the bridge
// reports parent-relative bounds). originX/Y is the parent's absolute origin.
// parentAutoLayout says whether n's own parent uses auto-layout (needed for
// the position:absolute structure-lint check — see figmaTarget).
// frameRenderOriginX/Y is the frame's own absoluteRenderBounds origin (0,0
// when geoDiff wasn't requested, in which case every renderBounds below
// stays nil anyway since n.RenderBounds itself wasn't fetched) — subtracted
// from each node's absolute render bounds to land in the same frame-relative
// space as box.
func collectTargets(n *figma.Node, originX, originY float64, root bool, parentAutoLayout bool, frameRenderOriginX, frameRenderOriginY float64, out map[string]figmaTarget) {
	absX, absY := originX+n.Bounds.X, originY+n.Bounds.Y
	if root {
		absX, absY = 0, 0 // frame is the coordinate origin
	}
	if t := tokensFromStyle(n.Styles); t != nil {
		out[n.ID] = figmaTarget{
			tokens: t, typ: n.Type, name: n.Name, text: n.Characters,
			box:               figma.Bounds{X: absX, Y: absY, Width: n.Bounds.Width, Height: n.Bounds.Height},
			layoutPositioning: styleLayoutPositioning(n.Styles),
			parentAutoLayout:  parentAutoLayout,
			renderBounds:      relativeRenderBounds(n.RenderBounds, frameRenderOriginX, frameRenderOriginY),
		}
	}
	childAutoLayout := n.Styles != nil && n.Styles.AutoLayout != nil
	for i := range n.Children {
		collectTargets(&n.Children[i], absX, absY, false, childAutoLayout, frameRenderOriginX, frameRenderOriginY, out)
	}
}

// relativeRenderBounds converts a node's absolute-page-coordinate render
// bounds into frame-relative space, or returns nil when render bounds
// weren't fetched for this node (geoDiff not requested).
func relativeRenderBounds(abs *figma.Bounds, frameOriginX, frameOriginY float64) *figma.Bounds {
	if abs == nil {
		return nil
	}
	return &figma.Bounds{
		X: abs.X - frameOriginX, Y: abs.Y - frameOriginY,
		Width: abs.Width, Height: abs.Height,
	}
}

// styleLayoutPositioning reads Figma's auto-layout escape hatch safely (nil
// Styles means "no layout info", not "absolute").
func styleLayoutPositioning(st *figma.Style) string {
	if st == nil {
		return ""
	}
	return st.LayoutPositioning
}

// tier1Diff is the deterministic core: for each Figma node aligned to a DOM
// element by data-figma-node, compare exact values within tolerance. Figma
// nodes with no DOM match are reported unmeasured (never silently passed).
func tier1Diff(want map[string]figmaTarget, got map[string]render.DOMElement) ([]ElementDiff, []UnmeasuredNode) {
	ids := make([]string, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var byElement []ElementDiff
	var unmeasured []UnmeasuredNode
	for _, id := range ids {
		ft := want[id]
		el, ok := got[id]
		if !ok {
			unmeasured = append(unmeasured, classifyUnmeasured(id, ft))
			continue
		}
		if diffs := compareNode(ft, el); len(diffs) > 0 {
			byElement = append(byElement, ElementDiff{NodeID: id, Name: ft.name, Diffs: diffs})
		}
	}
	return byElement, unmeasured
}

// classifyUnmeasured labels an unmatched target as actionable (the agent should
// tag and build it) or expected (decorative/image, not DOM-measurable).
func classifyUnmeasured(id string, ft figmaTarget) UnmeasuredNode {
	switch ft.typ {
	case "RECTANGLE", "ELLIPSE", "LINE", "VECTOR", "STAR", "POLYGON", "IMAGE", "BOOLEAN_OPERATION":
		return UnmeasuredNode{NodeID: id, Name: ft.name, Type: ft.typ,
			Actionable: false, Reason: "decorative/image node — not DOM-measurable"}
	default:
		return UnmeasuredNode{NodeID: id, Name: ft.name, Type: ft.typ,
			Actionable: true, Reason: "no data-figma-node match — tag this element to verify it"}
	}
}

// compareNode produces the field diffs for one aligned node/element pair.
func compareNode(ft figmaTarget, el render.DOMElement) []FieldDiff {
	t := ft.tokens
	var diffs []FieldDiff
	add := func(prop, is, should string) { diffs = append(diffs, FieldDiff{Prop: prop, Is: is, Should: should}) }
	addAdv := func(prop, is, should string) {
		diffs = append(diffs, FieldDiff{Prop: prop, Is: is, Should: should, Advisory: true})
	}
	cmp := func(prop, css string, want, tol float64) {
		if is, should, bad := cmpLen(css, want, tol); bad {
			add(prop, is, should)
		}
	}

	// opacity applies to any node type.
	if t.Opacity != nil {
		if is, should, bad := cmpScalar(el.Styles["opacity"], *t.Opacity, 0.02); bad {
			add("opacity", is, should)
		}
	}

	// Structure lint (anti-Goodhart): a DOM position:absolute inside a Figma
	// auto-layout parent is only legitimate when Figma itself declared this
	// node as an escape hatch (layoutPositioning=="ABSOLUTE"); otherwise the
	// design asked for a flex child and the implementation opted out of flow
	// on its own — flag it independent of whether pixels happen to match.
	if ft.parentAutoLayout && ft.layoutPositioning != "ABSOLUTE" {
		if pos := strings.TrimSpace(el.Styles["position"]); pos == "absolute" || pos == "fixed" {
			add("position", pos, "static (Figma auto-layout child)")
		}
	}

	if ft.typ == "TEXT" {
		if t.Fill != "" {
			if is, should, bad := cmpColor(el.Styles["color"], t.Fill); bad {
				add("color", is, withVariableHint(should, t.FillVariable))
			}
		}
		if t.FontSize != nil {
			cmp("font-size", el.Styles["font-size"], *t.FontSize, tolSize)
		}
		if t.FontWeight != nil {
			if is, should, bad := cmpNum(el.Styles["font-weight"], *t.FontWeight); bad {
				add("font-weight", is, should)
			}
		}
		if t.LineHeight != nil {
			cmp("line-height", el.Styles["line-height"], *t.LineHeight, tolSize)
		}
		if t.LetterSpacing != nil {
			// CSS "normal" letter-spacing means 0.
			ls := el.Styles["letter-spacing"]
			if strings.TrimSpace(ls) == "normal" {
				ls = "0px"
			}
			cmp("letter-spacing", ls, *t.LetterSpacing, tolSize)
		}
		if t.TextAlign != "" {
			if want, got := normalizeAlign(t.TextAlign), normalizeAlign(el.Styles["text-align"]); want != got {
				add("text-align", got, want)
			}
		}
		return diffs
	}

	if t.Fill != "" {
		if is, should, bad := cmpColor(el.Styles["background-color"], t.Fill); bad {
			add("background-color", is, withVariableHint(should, t.FillVariable))
		}
	}
	if t.Radius != nil {
		cmp("border-radius", el.Styles["border-top-left-radius"], *t.Radius, tolSize)
	}
	// Border: only when an actual stroke paint exists (the bridge reports
	// strokeWeight:1 even on borderless nodes, so gate on the stroke color).
	if t.Stroke != "" {
		if is, should, bad := cmpColor(el.Styles["border-top-color"], t.Stroke); bad {
			add("border-color", is, withVariableHint(should, t.StrokeVariable))
		}
		if t.StrokeWeight != nil {
			cmp("border-width", el.Styles["border-top-width"], *t.StrokeWeight, tolSize)
		}
	}
	if t.Gap != nil {
		domGap := firstNonEmpty(el.Styles["gap"], el.Styles["column-gap"], el.Styles["row-gap"])
		cmp("gap", domGap, *t.Gap, tolSpacing)
	}
	if t.Padding != nil {
		cmp("padding-top", el.Styles["padding-top"], t.Padding.Top, tolSpacing)
		cmp("padding-right", el.Styles["padding-right"], t.Padding.Right, tolSpacing)
		cmp("padding-bottom", el.Styles["padding-bottom"], t.Padding.Bottom, tolSpacing)
		cmp("padding-left", el.Styles["padding-left"], t.Padding.Left, tolSpacing)
	}
	// Drop shadow: report only a missing shadow (design has one, impl doesn't) —
	// matching exact shadow geometry is too noisy.
	if t.Shadow && !hasShadow(el.Styles["box-shadow"]) {
		add("box-shadow", "none", "drop shadow")
	}
	// Box size (containers only; text auto-sizes and would be noisy).
	if isTransformed(el.Styles["transform"]) {
		// getBoundingClientRect() is post-transform while our declared box
		// (Figma node.x/y/width/height) is pre-transform — comparing them
		// directly would be a guaranteed false diff, so a plain transform
		// skips this check entirely, same as before. Geo-diff (opt-in, see
		// Reconcile's geoDiff) replaces the skip with a real comparison:
		// Figma's own post-effects renderBounds against the DOM's
		// post-transform box. Both sides are "what actually rendered," so a
		// mismatch here localizes a transform *composition* bug (e.g. a
		// rotate applied around the wrong transform-origin) instead of
		// staying invisible because the declared values still match.
		if ft.renderBounds != nil {
			if is, should, bad := cmpDim(el.Box.X, ft.renderBounds.X, tolBox); bad {
				add("render-x", is, should)
			}
			if is, should, bad := cmpDim(el.Box.Y, ft.renderBounds.Y, tolBox); bad {
				add("render-y", is, should)
			}
			if is, should, bad := cmpDim(el.Box.Width, ft.renderBounds.Width, tolBox); bad {
				add("render-width", is, should)
			}
			if is, should, bad := cmpDim(el.Box.Height, ft.renderBounds.Height, tolBox); bad {
				add("render-height", is, should)
			}
		}
	} else {
		if ft.box.Width > 0 {
			if is, should, bad := cmpDim(el.Box.Width, ft.box.Width, tolBox); bad {
				addAdv("width", is, should)
			}
		}
		if ft.box.Height > 0 {
			if is, should, bad := cmpDim(el.Box.Height, ft.box.Height, tolBox); bad {
				addAdv("height", is, should)
			}
		}
	}
	return diffs
}

// isTransformed reports whether a computed transform is anything but identity.
func isTransformed(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && v != "none"
}

// hasShadow reports whether a computed box-shadow is present.
func hasShadow(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && v != "none"
}

// normalizeAlign maps Figma and CSS alignment values to a common form.
func normalizeAlign(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "left", "start", "":
		return "left"
	case "right", "end":
		return "right"
	case "center", "centre":
		return "center"
	case "justified", "justify":
		return "justify"
	default:
		return strings.ToLower(s)
	}
}

// --- canonicalization & comparison ---

// cmpLen compares a CSS length string against a px value within tol.
func cmpLen(css string, want, tol float64) (is, should string, bad bool) {
	should = fmt.Sprintf("%gpx", want)
	got, ok := parseLen(css)
	if !ok {
		return strings.TrimSpace(css), should, true
	}
	if math.Abs(got-want) > tol {
		return fmt.Sprintf("%gpx", got), should, true
	}
	return "", "", false
}

// cmpScalar compares a unitless CSS number (e.g. opacity) within tol.
func cmpScalar(css string, want, tol float64) (is, should string, bad bool) {
	should = strconv.FormatFloat(want, 'f', -1, 64)
	got, err := strconv.ParseFloat(strings.TrimSpace(css), 64)
	if err != nil {
		return strings.TrimSpace(css), should, true
	}
	if math.Abs(got-want) > tol {
		return strconv.FormatFloat(got, 'f', -1, 64), should, true
	}
	return "", "", false
}

// cmpDim compares two pixel dimensions (both already numeric) within tol.
func cmpDim(got, want, tol float64) (is, should string, bad bool) {
	if math.Abs(got-want) > tol {
		return fmt.Sprintf("%gpx", got), fmt.Sprintf("%gpx", want), true
	}
	return "", "", false
}

// cmpNum compares a numeric CSS value (e.g. font-weight) exactly.
func cmpNum(css string, want float64) (is, should string, bad bool) {
	should = strconv.FormatFloat(want, 'f', -1, 64)
	got, err := strconv.ParseFloat(strings.TrimSpace(css), 64)
	if err != nil {
		return strings.TrimSpace(css), should, true
	}
	if got != want {
		return strconv.FormatFloat(got, 'f', -1, 64), should, true
	}
	return "", "", false
}

// cmpColor compares a CSS color against a Figma hex, canonicalizing both.
func cmpColor(css, hex string) (is, should string, bad bool) {
	wantC, wok := canonColor(hex)
	gotC, gok := canonColor(css)
	should = hex
	if !wok {
		return "", "", false // can't assert if we can't parse the target
	}
	if !gok || gotC != wantC {
		return strings.TrimSpace(css), should, true
	}
	return "", "", false
}

// parseLen turns "16px" / "16" into 16.0.
func parseLen(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "px")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

// canonColor normalizes hex and rgb()/rgba() to "r,g,b,a" (a in 0..1, 2dp).
func canonColor(s string) (string, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "transparent":
		return "0,0,0,0.00", s == "transparent"
	}
	if strings.HasPrefix(s, "#") {
		h := strings.TrimPrefix(s, "#")
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) < 6 {
			return "", false
		}
		r, e1 := strconv.ParseInt(h[0:2], 16, 0)
		g, e2 := strconv.ParseInt(h[2:4], 16, 0)
		b, e3 := strconv.ParseInt(h[4:6], 16, 0)
		if e1 != nil || e2 != nil || e3 != nil {
			return "", false
		}
		return fmt.Sprintf("%d,%d,%d,1.00", r, g, b), true
	}
	if strings.HasPrefix(s, "rgb") {
		inner := s[strings.Index(s, "(")+1 : strings.LastIndex(s, ")")]
		parts := strings.Split(inner, ",")
		if len(parts) < 3 {
			return "", false
		}
		r, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		g, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		b, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
		a := 1.0
		if len(parts) >= 4 {
			a, _ = strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		}
		return fmt.Sprintf("%d,%d,%d,%.2f", r, g, b, a), true
	}
	return "", false
}

// withVariableHint appends the bound Figma Variable's name to a "should"
// value, when one exists — a hint, not an assertion: the tool can't tell
// from DOM computed styles whether the implementation used a literal or a
// token (getComputedStyle always returns the resolved value either way), so
// this only tells the agent a token is available to reach for, it doesn't
// claim the agent's code failed to use it.
func withVariableHint(should, variable string) string {
	if variable == "" {
		return should
	}
	return should + " (Figma Variable: " + variable + ")"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func hasMajor(fs []SemanticFinding) bool {
	for _, f := range fs {
		if f.Severity == "major" {
			return true
		}
	}
	return false
}

// reconcileImage is the no-DOM fallback: Tier-2 vision only.
func (s *Service) reconcileImage(ctx context.Context, key string, frame *figma.Node, imagePath string) (Diff, error) {
	rendered, err := os.ReadFile(imagePath)
	if err != nil {
		return Diff{}, err
	}
	findings, err := s.semanticDiff(ctx, key, frame, rendered, nil)
	if err != nil {
		return Diff{}, err
	}
	return Diff{Match: !hasMajor(findings), Semantic: findings}, nil
}

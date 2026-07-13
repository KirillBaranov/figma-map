# Limitations

> Honest gaps in the current release, not hidden behaviour. See the
> [README](../README.md) for what figma-map does; see [Roadmap](../README.md#roadmap)
> for which of these are actively being closed.

- **The binding is an AI draft.** `bind` infers prop values from story names
  using library conventions; it can miss an exact code value or invent a prop.
  **Review the binding** — that human-in-the-loop step is the design, not a bug.
- **Boolean props** are stringified (`disabled: ["false", "true"]`) and rendered
  as `disabled="true"` rather than the idiomatic bare `disabled`. Planned.
- **Import paths** come from the story source as written; relative imports stay
  relative. Adjust in the binding or normalize to your alias.
- **Static screenshots only** — hover/focus/active states are not observable
  from a screenshot, so a variant differing only by interaction state can't be
  distinguished by pixels. `figma animation <nodeId>` narrows this for nodes
  that actually have a prototyping reaction: it resolves the reaction's real
  destination when there is one (ground truth), or guesses a same-component
  state-sibling when there isn't (flagged `resolvedVia: "variant-sibling"`,
  not presented as designer-declared) — either way it's a best-effort style
  delta for the node you ask about, not general hover-state observation.
- **reconcile alignment** — design nodes are matched to DOM elements exactly via
  `data-figma-node` when present, otherwise by geometry/type/text so it works on
  an existing, untagged implementation (matched-by-position results are flagged
  lower-confidence). Unmatched nodes are reported `unmeasured`, never assumed
  correct. The goal is *spec-perfect* (every measured property matches the
  design), not pixel-raster identity, which font rendering makes unattainable.
- **reconcile property coverage**: color/background, font size/weight,
  line-height, letter-spacing, text-align, border radius/width/color, padding,
  gap, opacity, and element width/height. Not yet checked: margins, box-shadow,
  and gradient fills. Width/height can be content-driven, so treat those diffs as
  advisory.
- **Responsive is per-frame** — reconcile checks against one frame at the frame's
  width; behavior between breakpoints the design doesn't specify is out of scope.
- **Large documents are slow.** The backend won't kill a request outright for
  taking a while — the plugin heartbeats every few seconds and the backend's
  inactivity window resets on each one (`backend/src/bridge.ts`), so a large
  walk keeps running instead of getting cut off. But a full-styles walk
  (`get_node`/`get_document` resolving every node's styles + bound variables)
  over a document with a very large node count is still genuinely slow, not
  just theoretically safe from timing out. Today's mitigation is scoping
  calls with `--depth` where the caller doesn't need the whole subtree (see
  [Request flow](architecture.md#request-flow)); actually speeding up the
  walk is on the [Roadmap](../README.md#roadmap).
- **The bridge requires Figma desktop open** with the plugin running. For
  headless/CI/server-side agents, set `figma.source: rest` (`figma-map.example.yaml`)
  to use the Figma REST API instead (`figma.tokenEnv`, a Dev Mode/Enterprise-plan
  token) — additive, not a default, and strictly read-only: `find`/`inspect`/
  `tokens`/`screenshot`/`export-assets`/`map`/`plan`/`bind` work, but
  `capture issues`/`verify pixeldiff-images`/the compare loop do **not** — those
  require a live DOM and a live Figma document in sync, which a static REST
  snapshot can't provide (`capture issues`/`ack` fail with a clear
  "issue inbox requires a bridge connection" rather than silently no-op'ing).
  `Selection` similarly errors instead of returning an empty result — the REST
  API has no concept of "what's currently selected in the editor." A handful of
  Node fields the bridge fills from a live document (bound-variable resolution
  beyond fills/strokes, prototyping reactions, dev-resources, annotations, grid
  position) aren't mapped from REST yet — left absent, never fabricated.
  `figma animation` errors the same way `Selection` does — resolving a
  reaction's before/after state needs the bridge/plugin.

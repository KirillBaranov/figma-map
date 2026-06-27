---
name: figma-map
description: Use when implementing UI from a Figma design — building a page or component from a frame/mockup, pulling exact design tokens (color/spacing/font) or Figma Variables, mapping Figma components to the code library, or verifying that rendered output matches the design. Drives the figma-map tool (CLI or MCP).
---

# figma-map: build UI from Figma, verified by numbers

figma-map turns implementing a Figma design into a **closed loop**: you write
code, it measures your rendered output against the design's exact values, you fix
the numbers it reports, repeat until they match. It is a *dumb tool* — it
measures and supplies data; **you (the agent) own the loop and write the code.**

Core idea: anything with a ground-truth value (size, color, spacing, font,
bound Variable) is **computed, not guessed**. Reconcile compares Figma's exact
tokens against your DOM's computed styles and returns per-element
`is → should` numbers.

## Invocation: MCP tools or CLI

Commands are grouped by what they do. The CLI nests as
`figma-map <group> <verb>`; MCP exposes the same op as a flat `group_verb`
tool name (e.g. `figma_find`, `build_plan`). Both are generated from one
registry, so names and parameters can't drift.

- If the **figma-map MCP server** is connected, call tools by their flat name
  (`build_plan`, `verify_reconcile`, `figma_tokens`, …).
- Otherwise use the **CLI** and add `--json` for machine-readable output:
  `figma-map build plan 55:1102 --json`.

Groups: **figma** (read ground truth — find/inspect/selection/pages/tokens/variables),
**capture** (images — screenshot/render/export), **build** (codegen/map/plan),
**verify** (pixeldiff/reconcile), **setup** (scan/bind/components). `doctor` and
`mcp` are ungrouped.

## Before you start

Run `doctor` (ungrouped, no group prefix). It checks the Figma bridge, headless
Chrome, Storybook, and the API key — and separately reports whether a Figma
file is actually connected to the bridge (the bridge process can be up with no
plugin connected; that shows as its own failing check, not a generic
"unreachable"). The design must be open in Figma with the bridge plugin
running. A `figma-map.binding.yaml` + `catalog/` must exist (created once by
`setup scan` then `setup bind`); if missing, ask the user to run them.

No node id yet? Call **`figma pages`** first — file name + page list, no tree,
no styles. Then **`figma find`**/**`figma inspect`** to drill in.

Deterministic operations (`doctor`, `figma tokens`, `figma inspect`,
`capture screenshot`, `capture export`, `setup components`, `verify reconcile`
Tier-1) need **no API key**. Only `setup bind`, `build map`, `build plan`, and
`verify reconcile --semantic` use the LLM.

## The build loop (e.g. "build this page from frame 55:1102")

1. **`build plan <nodeId>`** — get the buildable spec: container layout, each
   component instance mapped to your code (`import` + `symbol` + `props`), exact
   `tokens`, and an honest `unmapped` list. This is your blueprint.

2. **Write the code** from the plan:
   - Use the mapped components with their `import`/`props`.
   - Build `unmapped` pieces by hand using their `tokens` (exact values,
     including `fillVariable`/`strokeVariable`/`variables` when a value is
     bound to a Figma Variable rather than a literal — name the value after
     the variable, don't hardcode it).
   - **Stamp every element you create with `data-figma-node="<id>"`** using the
     node id from the plan. This is the contract that lets reconcile measure your
     output. Untagged elements are reported `unmeasured` (not assumed correct).
   - For images/icons, use **`capture export <nodeId> --format svg`** — export
     the real asset; never regenerate it. (`build codegen` also auto-exports
     vector/icon nodes to SVG files and emits `<img>` tags for you.)

3. **Render** your implementation — a Storybook story or a dev-server URL.

4. **`verify reconcile <nodeId> --story <storyId>`** (or `--url <url>`). It
   renders your output at the frame's width, reads the DOM, and diffs against
   the design:

   ```jsonc
   { "match": false, "remaining": 2, "byElement": [
       { "nodeId": "55:1140", "name": "CTA", "diffs": [
           { "prop": "background-color", "is": "rgb(31,41,55)", "should": "#18181b" },
           { "prop": "padding-left", "is": "12px", "should": "16px" } ] } ],
     "unmeasured": ["55:1150"] }
   ```

5. **Fix exactly what it reports** (set background to `#18181b`, padding-left to
   16px), re-render, and reconcile again. Loop until `"match": true`.
   - `unmeasured` ids mean you forgot a `data-figma-node` tag — add it.
   - Add `--semantic` for an LLM pass on missing elements / wrong assets that
     numbers can't catch (returns `semantic` findings; needs the API key).
   - No real implementation yet? **`verify pixeldiff <nodeId>`** without `--url`
     diffs against figma-map's own raw codegen render instead. Read its
     `regions` field (a worst-first grid of per-cell diff%) to find *where*
     things differ — don't try to visually interpret the diff image.

`match: true` means every measured property is within tolerance (spec-perfect).
Pixel-raster identity is **not** the goal — font rendering makes it unattainable
and a human dev can't hit it either.

## Other operations

- **`figma tokens <nodeId>`** — exact normalized tokens for one node (color,
  padding, gap, radius, font, plus `fillVariable`/`strokeVariable`/`variables`
  for Figma Variable bindings and `reactions` for prototyping transitions).
  Use when hand-building or to double-check a value.
- **`figma variables`** — the file's full Variable catalog (every
  collection/variable/mode), independent of any node. Use to see what tokens
  exist; `figma tokens` tells you what a *specific* node is bound to.
- **`figma inspect <nodeId> [--tokens] [--depth N]`** — the node subtree
  (structure, text, bounds, optionally tokens/reactions/devResources/annotations).
  Use to understand a design's shape.
- **`figma find <query>`** — search nodes by name/text/type; surfaces `devStatus`
  so you can filter to frames actually marked ready for dev.
- **`figma selection`** — the node(s) currently selected in the Figma editor.
- **`build map <nodeId>`** — identify a single node's component and emit JSX. Use
  for a one-off component rather than a whole frame.
- **`capture screenshot <nodeId>`** / **`capture render <nodeId>`** — render a
  node to an image. Both write to a default path
  (`.figma-map/out/<nodeId>-<kind>.png`) and return that path — pass `--inline`
  only if you actually need the bytes back in the response.
- **`setup components`** — the components available in the binding.

## Rules

- **You write and fix the code; figma-map never does.** It answers and measures.
- **Always tag generated elements** with `data-figma-node` — it's what makes the
  loop work. Build from `build plan` so the ids are at hand.
- **Trust the numbers over your eyes.** If reconcile says padding is 12 vs 16,
  it's 12 vs 16 — fix it, don't re-examine the screenshot. Same for pixeldiff's
  `regions` breakdown — read the numbers, don't eyeball the overlay image.
- **Name values after their bound Variable when one exists.** `figma tokens`
  reports both the literal value and (via `fillVariable`/`strokeVariable`/
  `variables`) the Variable it's bound to, if any — use the Variable name,
  not the literal, since that's the actual design-system token.
- **Don't stop at "looks right."** Stop at `match: true` (or only acceptable
  `unmeasured`/cosmetic items remain, confirmed with the user).
- The binding is a reviewed artifact; if `build map`/`build plan` pick a wrong
  component or prop, tell the user — the fix belongs in `figma-map.binding.yaml`.

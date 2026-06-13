---
name: figma-map
description: Use when implementing UI from a Figma design — building a page or component from a frame/mockup, pulling exact design tokens (color/spacing/font), mapping Figma components to the code library, or verifying that rendered output matches the design. Drives the figma-map tool (CLI or MCP).
---

# figma-map: build UI from Figma, verified by numbers

figma-map turns implementing a Figma design into a **closed loop**: you write
code, it measures your rendered output against the design's exact values, you fix
the numbers it reports, repeat until they match. It is a *dumb tool* — it
measures and supplies data; **you (the agent) own the loop and write the code.**

Core idea: anything with a ground-truth value (size, color, spacing, font) is
**computed, not guessed**. Reconcile compares Figma's exact tokens against your
DOM's computed styles and returns per-element `is → should` numbers.

## Invocation: MCP tools or CLI

- If the **figma-map MCP server** is connected, call its tools directly. Tool
  names equal the operation names below (`plan`, `reconcile`, `tokens`, …).
- Otherwise use the **CLI** and add `--json` for machine-readable output:
  `figma-map plan 55:1102 --json`.

Both are generated from one registry, so names and parameters are identical.

## Before you start

Run `doctor` (or `figma-map doctor`). It checks the Figma bridge, headless
Chrome, Storybook, and the API key. The design must be open in Figma with the
bridge plugin running. A `figma-map.binding.yaml` + `catalog/` must exist (created
once by `scan` then `bind`); if missing, ask the user to run them.

Deterministic operations (`doctor`, `tokens`, `inspect`, `screenshot`,
`export-assets`, `list`, `reconcile` Tier-1) need **no API key**. Only `bind`,
`map`, `plan`, and `reconcile --semantic` use the LLM.

## The build loop (e.g. "build this page from frame 55:1102")

1. **`plan <frameId>`** — get the buildable spec: container layout, each
   component instance mapped to your code (`import` + `symbol` + `props`), exact
   `tokens`, and an honest `unmapped` list. This is your blueprint.

2. **Write the code** from the plan:
   - Use the mapped components with their `import`/`props`.
   - Build `unmapped` pieces by hand using their `tokens` (exact values).
   - **Stamp every element you create with `data-figma-node="<id>"`** using the
     node id from the plan. This is the contract that lets reconcile measure your
     output. Untagged elements are reported `unmeasured` (not assumed correct).
   - For images/icons, use **`export-assets <nodeId> --format svg`** — export the
     real asset; never regenerate it.

3. **Render** your implementation — a Storybook story or a dev-server URL.

4. **`reconcile <frameId> --story <storyId>`** (or `--url <url>`). It renders your
   output at the frame's width, reads the DOM, and diffs against the design:

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

`match: true` means every measured property is within tolerance (spec-perfect).
Pixel-raster identity is **not** the goal — font rendering makes it unattainable
and a human dev can't hit it either.

## Other operations

- **`tokens <nodeId>`** — exact normalized tokens for one node (color, padding,
  gap, radius, font). Use when hand-building or to double-check a value.
- **`inspect <nodeId> [--tokens] [--depth N]`** — the node subtree (structure,
  text, bounds). Use to understand a design's shape.
- **`map <nodeId>`** — identify a single node's component and emit JSX. Use for a
  one-off component rather than a whole frame.
- **`screenshot <nodeId> [--out f.png]`** — render a node to an image (handy to
  look at a specific element).
- **`list`** — the components available in the binding.

## Rules

- **You write and fix the code; figma-map never does.** It answers and measures.
- **Always tag generated elements** with `data-figma-node` — it's what makes the
  loop work. Build from `plan` so the ids are at hand.
- **Trust the numbers over your eyes.** If reconcile says padding is 12 vs 16,
  it's 12 vs 16 — fix it, don't re-examine the screenshot.
- **Don't stop at "looks right."** Stop at `match: true` (or only acceptable
  `unmeasured`/cosmetic items remain, confirmed with the user).
- The binding is a reviewed artifact; if `map`/`plan` pick a wrong component or
  prop, tell the user — the fix belongs in `figma-map.binding.yaml`.

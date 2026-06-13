# ADR-0001: figma-map is a dumb tool — deterministic-first, agent owns the loop

- Status: accepted
- Date: 2026-06-13

## Context

figma-map exists to let AI coding agents build UI from Figma designs reliably —
ideally autonomously (point at a frame, get a spec-perfect page). Naive agents are
unreliable at this because they *guess from pixels* and have no objective notion
of "done": they stop when output looks plausible.

We need to decide what figma-map is responsible for, and — critically — where a
language model is allowed to make decisions versus where exact computation must
rule.

## Decision

**figma-map is a dumb tool: a connector, a measurement engine, and a black box
for the few irreducibly-fuzzy judgments. It has no brains and no loop.**

1. **No brains, no loop.** figma-map connects (Figma via the bridge, the rendered
   DOM via headless Chrome, the code catalog), extracts exact data, **measures**,
   and exports assets. It never writes code, never decides what to build, never
   iterates.
2. **The agent owns the loop.** Codegen, iteration, convergence control, plateau
   detection, and "good enough / escalate to human" all live in the agent
   harness. figma-map only *answers questions and measures*.
3. **Deterministic-first.** Anything with ground truth is computed, not guessed.
   The Figma node tree gives exact "should-be" values; the rendered DOM gives
   exact "is" values. A difference that is a number is measured, never eyeballed.
4. **LLM only for the irreducibly fuzzy, and encapsulated.** Where no amount of
   math decides (e.g. "which catalog component does this region look like",
   "is an element missing / is this the wrong icon"), figma-map may use a vision
   LLM — but behind a typed tool that returns structured data. That an LLM is
   used is figma-map's private implementation detail; the agent just calls a tool.
   This surface is kept as small as possible.
5. **The tool does all screenshotting and image handling.** The expensive coding
   model never captures images, and in the reconcile loop never *consumes* them —
   it receives a structured numeric diff. Vision tokens are spent only inside
   figma-map's cheap, encapsulated check.
6. **Deterministic operations require no API key.** The LLM client is built
   lazily; only the fuzzy operations need credentials.

## Per-operation split (deterministic vs LLM)

| Operation | Deterministic part | LLM part (encapsulated) |
|---|---|---|
| `doctor` | env checks | — |
| `scan` | screenshot stories, parse imports | — |
| `tokens` | exact color/spacing/font/radius from the node tree | — |
| `inspect` | node tree (id/name/type/text/bbox/tokens) | component identity *iff* `--annotate` |
| `screenshot` | render a node to an image | — |
| `export-assets` | export image/vector nodes to files | — |
| `list` | read the binding | — |
| `map` | codegen JSX, prop values derivable from the tree | which component (visual identity) |
| `plan` | walk frame, dedupe, layout from autolayout, tokens | which component per instance |
| `bind` | catalog/section handling, write the binding | section → component matching |
| `reconcile` Tier 1 | Figma tokens ↔ DOM computed styles, with tolerances | — |
| `reconcile` Tier 2 | — | missing element / wrong asset / gestalt |
| `reconcile` Tier 3 (later) | pixel diff for the no-DOM case | optional segmentation (SAM) |

## Consequences

- Reliability comes from the **closed loop with a deterministic oracle**, not from
  a smarter model. The agent gets an objective error signal (numeric diff) and an
  objective stopping criterion (error within tolerance).
- Every property moved from "LLM guesses" to "compute the difference" is one fewer
  source of flakiness, N fewer tokens per iteration, and one more unit test.
- The reconcile loop converges against **tolerances** (sub-pixel and font-metric
  differences mean exact-zero is unattainable and undesirable); the target is
  *spec-perfect* (every measurable property matches the design), not pixel-raster
  identity.
- figma-map stays small, testable, and provider-agnostic (`llm.baseURL` → local
  model). It is a tool, not a product that locks anyone in.
- Things the design does not specify (interaction states, behavior between
  breakpoints) cannot be reconciled and are surfaced honestly, not invented.

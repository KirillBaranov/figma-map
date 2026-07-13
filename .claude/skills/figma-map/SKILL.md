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

The bridge is a separate Node process + a Figma plugin — not part of *this*
project's own code even when `figma-map` is being used from inside it. It
lives in the `figma-map` source repo (wherever the user cloned/pulled it —
check with them if you don't already know the path), listens on **`:1994`**
by default (the `bridge:` URL in `figma-map.yaml` must match), and needs two
things running before any `figma`/`build`/`verify` call will work:

1. **The backend process.** Try **`bridge up`** first (ungrouped, no group
   prefix) — it pings the configured bridge URL, does nothing if something's
   already there, otherwise builds and starts the backend for you and reports
   back once it's actually answering. It needs to know where the figma-map
   repo is: pass `--repo <path>`, or use it without a flag if `bridgeRepo` is
   already set in `figma-map.yaml`. If neither is set, it errors with exactly
   what to do — ask the user for the repo path once, or fall back to running
   it by hand: `npm --prefix backend run build && node backend/dist/index.js`
   from that repo. `bridge status`/`bridge down` check/stop it the same way.
2. **The Figma plugin**, in Figma itself: **Plugins → Development → Import
   plugin from manifest**, pointing at `extensions/plugin/manifest.json` in
   that same repo, run against the open design. `bridge up` can't do this
   part — it's a one-time manual step per Figma session, same as opening the
   file itself.

If `doctor` fails on the bridge/plugin checks below, these are the two
things to check first — don't assume a code bug.

No node id yet? Call **`figma pages`** first — file name + page list, no tree,
no styles. Then **`figma find`**/**`figma inspect`** to drill in.

Deterministic operations (`doctor`, `figma tokens`, `figma inspect`,
`capture screenshot`, `capture export`, `setup components`, `verify reconcile`
Tier-1) need **no API key**. Only `setup bind`, `build map`, `build plan`, and
`verify reconcile --semantic` use the LLM.

## Troubleshooting: operational failures

These aren't bugs in figma-map — they're the normal failure modes of driving
a real Figma tab over a WebSocket bridge. Recognize them by message and
react accordingly instead of guessing or giving up.

- **"plugin unresponsive (no ack received — window may be unfocused or
  suspended by Figma)"** — the single most common failure. Browsers/Figma
  suspend JS execution in backgrounded or unfocused tabs; the bridge's
  WebSocket connection stays open but the plugin can't respond. The bridge
  already retries once internally before surfacing this — if you're seeing
  it at all, the tab was unfocused for the whole retry window, not a one-off
  blip. **Fix: ask the user to click into the Figma tab/window so it's
  focused, then re-issue the exact same call.** Don't route around it by
  switching to a different operation or assuming the data is wrong.
- **`doctor` failing — read *which* check, not just that it failed.**
  `figma bridge unreachable` and `figma plugin connected` are reported as
  two separate checks on purpose, because they look identical to a human but
  need different fixes:
  - `figma bridge unreachable — restart it: cd backend && node dist/index.js`
    — the backend process itself isn't running. Run **`bridge up`** (with
    `--repo` if `bridgeRepo` isn't set) before retrying anything — see
    "Before you start" above.
  - `bridge is up but no Figma file is connected — open the file and run the
    plugin in Figma (Plugins → Development)` — the backend is fine; the
    Figma plugin just isn't running in a tab yet, or the tab was closed.
    Ask the user to open the design and start the plugin, then rerun
    `doctor`.
  - `storybook (...)` failing is unrelated to the Figma side — only
    `build codegen`'s live-render comparisons need it; ignore it for
    pure `figma`/`build`/`verify reconcile` (Tier-1) work.
- **A large `figma find`/`figma inspect` on a big page looks stuck but isn't
  — the bridge tolerates real silence up to ~90s** (it watches for a live
  progress heartbeat, not overall duration) before it would actually time
  out and report a diagnosis. A long wait with no error yet is expected on
  a large selection, not a hang to interrupt.
- **`Port 1994 already in use` / a stale backend process** — only one
  backend process can hold the bridge port; a leftover process from a
  previous run (crashed agent session, forgotten background job) can block
  a fresh one from starting. `bridge down` (stops what `bridge up` started,
  by its recorded pid) then `bridge up` again is the clean way to replace
  it. If that process wasn't started by `bridge up` (no pidfile — e.g. a
  human ran it by hand), find and stop it manually instead:
  ```bash
  lsof -nP -iTCP:1994 -sTCP:LISTEN   # find the PID holding it
  kill <pid>
  ```
  then `bridge up` (or `node backend/dist/index.js &` from the figma-map
  repo) to start a fresh one. Either way, don't assume the port itself is
  broken.
- Don't loop blindly retrying a failing call more than once. The bridge
  already retries transient network blips internally and invisibly — if an
  error still reaches you, retrying the identical call without addressing
  the underlying cause (focus, a dead backend, a stale port) will just fail
  the same way again. Diagnose from the message first.

## The build loop (e.g. "build this page from frame 55:1102")

1. **`build plan <nodeId>`** — get the buildable spec: container layout, each
   component instance mapped to your code (`import` + `symbol` + `props`), exact
   `tokens`, and an honest `unmapped` list. Each entry also carries a `jsx`
   field — a matched instance's `jsx` is the ready element in your library's
   own format (e.g. `<Button variant="primary">Start</Button>`, built from
   real prop values read off that instance); an unmapped instance's `jsx` is
   the same raw div/span skeleton `build codegen` would emit, a starting
   point rather than bare tokens to compose by eye. This is your blueprint.

2. **Write the code** from the plan:
   - Use the mapped components' `jsx` (or `import`/`symbol`/`props` directly).
   - Build `unmapped` pieces from their `jsx` skeleton, filling in `tokens`
     (exact values, including `fillVariable`/`strokeVariable`/`variables`
     when a value is bound to a Figma Variable rather than a literal — name
     the value after the variable, don't hardcode it).
   - `build codegen`'s skeleton (and an unmapped instance's `jsx`) always uses
     inline `style={{...}}` with literal px/hex values — that's a structural
     scaffold, not final code. Convert it to your project's real styling
     convention (Tailwind classes, CSS modules, design tokens, etc.) before
     treating it as done; don't ship inline styles into a codebase that
     doesn't otherwise use them.
   - Same for `position: absolute` + explicit `left`/`top`: codegen emits
     that for any frame that isn't using Figma auto-layout — a literal
     mirror of where the designer happened to drop things on the canvas,
     not a layout recommendation. Ship it verbatim and you get a page that
     matches the screenshot but doesn't survive a text change, a translation,
     or a resize. Look at what's actually there: siblings that read as a row
     or column (most of them, even when the designer never turned on
     auto-layout) should become a normal-flow flex/grid container; reserve
     `position: absolute` for elements that genuinely need to float or
     overlap (badges, decorative overlays, pinned corners). The spacing/size
     *values* are still ground truth either way (from `tokens`/the plan) —
     only the CSS mechanism expressing them is your judgment call, not
     Figma's, and not something figma-map can decide for you (no
     structure-guessing heuristics — that's exactly the kind of judgment call
     that's yours, not the tool's).
     Figma's canvas has no concept of "row" or "overlap is a mistake" — a
     designer drops decorative elements (illustration clusters, badges, glow
     blobs) wherever they look right, deliberately overlapping or bleeding
     past a frame's edge, precisely *because* nothing downstream needs to
     reflow them. A true row/column, by contrast, exists because content
     flows through it (text length varies, items get added/removed) —
     that's the thing flex is *for*. So the question isn't "do these look
     aligned" but "if the content changed, would a human have wanted these
     to reflow, or would that break the composition." Bounds that overlap
     heavily or spill past their parent are a symptom worth noticing, not a
     threshold to hit — one piece of evidence pointing at the same
     conclusion the designer's intent already implies once you look for it.
     When it's genuinely unclear, a screenshot compare (`verify pixeldiff
     --selector`) against the Figma render is worth more here than
     iterating on numbers: reconcile checks property values, not whether
     you picked the right container mechanism, so a wrong call here sails
     through every per-property check and still looks wrong.
   - **Stamp every element you create with `data-figma-node="<id>"`** using the
     node id from the plan. This is the contract that lets reconcile measure your
     output. Untagged elements are reported `unmeasured` (not assumed correct).
   - For images/icons, use **`capture export <nodeId> --format svg`** — export
     the real asset; never regenerate it. (`build codegen` also auto-exports
     vector/icon nodes to SVG files and emits `<img>` tags for you.)
     This matters most for icon-kit illustration instances with complex or
     visually-rotated vector geometry: that tilt is frequently baked
     directly into the path data rather than expressed as a `rotation`
     transform on the node, so there's no clean number to hand-translate.
     Redrawing it from a generic icon set is redrawing a *different* asset
     that happens to look similar, not the same shape — and it's a
     synchronization hazard worth surfacing to the user rather than
     silently "fixing": if the Figma source instance changes later, a
     hand-recreated stand-in has no path back to the original and will
     quietly drift out of sync.

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
   - Building a section that lives mid-page rather than as its own isolated
     story? Add **`--selector <nodeId-or-css-selector>`** to scope the
     screenshot to just that element instead of the whole viewport — no need
     to spin up isolation for it first. This is the concrete tool behind the
     screenshot-before-committing-to-a-layout-mechanism note above, e.g.
     `verify pixeldiff 1232:33509 --url http://localhost:3000/page --selector 1232:33509`.

`match: true` means every measured property is within tolerance (spec-perfect).
Pixel-raster identity is **not** the goal — font rendering makes it unattainable
and a human dev can't hit it either.

## Other operations

- **`figma tokens <nodeId>`** — exact normalized tokens for one node (color,
  padding, gap, radius, font, plus `fillVariable`/`strokeVariable`/`variables`
  for Figma Variable bindings and `reactions` for prototyping transitions).
  Use when hand-building or to double-check a value.
- **`figma animation <nodeId>`** — for a node with `reactions`, resolves what
  actually changes: `figma tokens`' `reactions` only gives trigger/timing
  (cheap, on every node); this does the expensive part — following a
  reaction's destination (or, when there isn't one, guessing a same-component
  state-sibling and saying so via `resolvedVia`) and diffing styles into
  `styleDelta.{from,to}`. Use those values to write a real CSS
  `transition`/framer-motion `animate`, not just a note that something
  hovers.
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
- **`capture browser <url> [--selector]`** — the implementation-side
  counterpart: screenshot a live URL (dev server, Storybook iframe, local
  HTML file), the whole viewport or, with `--selector` (CSS selector or a
  bare Figma node id), cropped to one element inside a normally-sized page —
  for looking at what's currently rendered without comparing it against a
  Figma node (`verify pixeldiff --selector` does the same crop internally
  when you *are* comparing). Same output convention: writes a PNG to
  `--out`/a default path, `--inline` for the bytes.
- **`setup components`** — the components available in the binding.

## Rules

- **You write and fix the code; figma-map never does.** It answers and measures.
  Its outputs — `build plan`'s `jsx`, `build codegen`'s skeleton, `figma
  tokens`' values, a screenshot — are material to work from, not a
  deliverable to ship. You're the one who answers for the final code, which
  means it's your job to make it match how *this* project actually does
  things (its component library, its styling convention, its file/naming
  patterns) — figma-map has no way to know that and isn't trying to. A
  rough shape of the loop: pull a screenshot of the target (the Figma
  selection, or the matching element once something's rendered) → read the
  plan/tokens to understand what's actually being built and with what exact
  values → take the generated snippet as a reference shape, not a patch to
  apply → rewrite it to fit the codebase's existing patterns → verify with
  pixeldiff/reconcile → escalate to the user instead of guessing when
  something is genuinely ambiguous (a binding that picked the wrong
  component, a design decision numbers can't settle).
- **Always tag generated elements** with `data-figma-node` — it's what makes the
  loop work. Build from `build plan` so the ids are at hand.
- **Trust the numbers over your eyes.** If reconcile says padding is 12 vs 16,
  it's 12 vs 16 — fix it, don't re-examine the screenshot. Same for pixeldiff's
  `regions` breakdown — read the numbers, don't eyeball the overlay image.
  This holds for *property values*, which have one correct number and
  reconcile already checks mechanically. It doesn't extend to *structural*
  decisions like flex vs absolute (see the `position: absolute` note above)
  — that's not a value reconcile measures at all, so a wrong container
  mechanism will pass every property check and still be visibly wrong.
  When a section's composition isn't obviously a clean flow, a screenshot
  compare is the fastest way to sanity-check the structural read before
  sinking time into numeric fixes that can't catch that class of mistake.
- **Name values after their bound Variable when one exists.** `figma tokens`
  reports both the literal value and (via `fillVariable`/`strokeVariable`/
  `variables`) the Variable it's bound to, if any — use the Variable name,
  not the literal, since that's the actual design-system token.
- **Don't stop at "looks right."** Stop at `match: true` (or only acceptable
  `unmeasured`/cosmetic items remain, confirmed with the user).
- **`position: absolute` everywhere is a tell, not a target.** It means you
  (or codegen's literal scaffold) skipped converting real rows/columns into
  flex/grid. It'll pass reconcile/pixeldiff — both measure values, not layout
  resilience — and still be wrong: the result matches the screenshot but
  breaks the moment text wraps differently, a translation is longer, or the
  viewport resizes. A human should be able to touch the page (resize it,
  retype the copy) without it falling apart.
- The binding is a reviewed artifact; if `build map`/`build plan` pick a wrong
  component or prop, tell the user — the fix belongs in `figma-map.binding.yaml`.

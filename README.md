<div align="center">

# figma-map

**Map Figma design components to your code component library using a vision LLM.**

Match once with AI into a reviewable binding, then generate code deterministically.

[![CI](https://github.com/KirillBaranov/figma-map/actions/workflows/ci.yml/badge.svg)](https://github.com/KirillBaranov/figma-map/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kirillbaranov/figma-map.svg)](https://pkg.go.dev/github.com/kirillbaranov/figma-map)
[![Go Report Card](https://goreportcard.com/badge/github.com/kirillbaranov/figma-map)](https://goreportcard.com/report/github.com/kirillbaranov/figma-map)
[![Release](https://img.shields.io/github/v/release/KirillBaranov/figma-map?sort=semver)](https://github.com/KirillBaranov/figma-map/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

---

figma-map answers a question every design-system team hits: *"which `<Button>` is
this Figma layer, and with what props?"* — automatically.

Instead of hand-maintaining a mapping between your design library in Figma and
your component library in code, figma-map builds that mapping **once** with a
vision LLM, lets you **review** it, then applies it **deterministically** to
generate JSX.

```text
// 13:1077 → Button (0.80)
import { Button } from "@/components/ui/button"

<Button>View docs</Button>
```

## Features

- **AI runs once, codegen runs forever.** `bind` matches your Figma library to
  your code library with a vision LLM a single time, into a reviewable
  `figma-map.binding.yaml`. Every `map`/`plan`/`codegen` call after that is
  plain, repeatable, CI-friendly code generation — no LLM in the hot path.
- **Ground truth before vision.** Component identity and prop values are read
  straight from Figma — instance name, main-component name, `componentProps`,
  bound-Variable `codeSyntax` — whenever Figma already has the answer. The
  vision model is the fallback, not the default.
- **Exact verification, not "looks right."** `verify reconcile` renders your
  implementation, reads its actual DOM, and diffs computed styles against the
  design's exact tokens: per-element `is → should` numbers, not a screenshot
  for you to eyeball. `verify pixeldiff` adds a worst-region breakdown for
  anything text-diffing can't catch.
- **MCP-native.** Every CLI command is generated from the same registry as an
  MCP tool (`figma_find`, `build_plan`, …) — point an agent at it and it gets
  the identical surface, flags included, with zero drift between the two.
- **No Figma REST rate limits.** A bridge plugin running inside your open
  Figma file talks to figma-map directly over a local WebSocket — no API
  token, no per-minute request ceiling, no waiting on Figma's API quota.
- **Honest about what it can't do.** Unmatched components, untagged DOM
  elements, and unresolved props are reported explicitly (`unmapped`,
  `unmeasured`, a vision fallback that still failed) — never silently guessed.

## Contents

- [Features](#features)
- [Why](#why)
- [How it works: bind → apply](#how-it-works-bind--apply)
- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [Agent / MCP integration](#agent--mcp-integration)
- [Configuration](#configuration)
- [Architecture](#architecture)
- [Limitations](#limitations)
- [Contributing](#contributing)
- [License](#license)

## Why

Translating a Figma design into code means repeatedly recognizing *"this is our
Button, in the secondary variant, large size"* and writing the matching JSX.
That recognition is mechanical but tedious, and it drifts as the design system
evolves.

figma-map treats the design library and the code library as two sets of images
and learns the correspondence between them — the same way you would by eye, but
captured as a committable artifact instead of living in someone's head.

## How it works: bind → apply

The tool is built around a reviewable artifact, `figma-map.binding.yaml`, the
same way codegen and i18n tools work. The AI runs **once**, during `bind`;
everything downstream is deterministic and CI-friendly.

```text
  Storybook ──scan──▶  catalog/            (screenshots + import paths, no AI)
                          │
  Figma ────────────────┐ │
                        ▼ ▼
                  bind  (vision LLM, once)  ──▶  figma-map.binding.yaml
                                                       │  ← you review it
                                                       ▼
  Figma node ──── map (deterministic) ───────────▶  JSX
```

1. **`scan`** — screenshot every Storybook story into a code-component catalog.
2. **`bind`** *(AI, once)* — match each Figma component section to the catalog
   and infer each component's prop schema → write `figma-map.binding.yaml`.
3. **review the binding** — it is a draft; correct anything the model got wrong.
4. **`map`** *(cheap, repeatable)* — for any Figma node, identify its component
   and prop values from the binding and emit JSX.

## Install

One line — detects your OS/arch, downloads the matching release, verifies its
SHA-256 checksum, and installs the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/KirillBaranov/figma-map/main/install.sh | sh
```

Overrides: `FIGMA_MAP_VERSION=v0.1.0` to pin a tag, `FIGMA_MAP_INSTALL_DIR=~/bin`
to choose the directory.

With Go:

```bash
go install github.com/kirillbaranov/figma-map@latest
```

Or download a prebuilt archive from the
[releases page](https://github.com/KirillBaranov/figma-map/releases).

### Requirements

| Dependency | Why |
|---|---|
| **Google Chrome / Chromium** | headless screenshots of Storybook stories |
| **Storybook 7+** running | exposes the `index.json` story manifest |
| **backend + bridge/plugin running** (backend is a vendored fork of [gethopp/figma-mcp-bridge](https://github.com/gethopp/figma-mcp-bridge)'s server, see [bridge/NOTICE.md](bridge/NOTICE.md)) | connects an open Figma file to a local server on `:1994`, bypassing Figma API rate limits — start with `npm --prefix backend run build && node backend/dist/index.js`, then load the plugin in Figma (Plugins → Development → Import from manifest, `bridge/plugin/manifest.json`) |
| **OpenAI-compatible vision endpoint + key** | matching and prop inference (works with OpenAI, a local Ollama/llava server, or any compatible gateway via `llm.baseURL`) |

## Quick start

```bash
cp figma-map.example.yaml figma-map.yaml      # adjust URLs if needed
export OPENAI_API_KEY=sk-...

figma-map doctor                              # verify bridge, chrome, storybook, key

# 1. Build the code-component catalog (no AI).
#    --project points at the repo containing your *.stories.tsx files.
figma-map setup scan --project /path/to/storybook-project

# 2. Match Figma to the catalog and write the binding (AI, run once).
figma-map setup bind
#    → review figma-map.binding.yaml

# 3. Generate code for any Figma node.
figma-map build map 13:1077
```

## Commands

Commands are grouped by what they do — `figma-map <group> <verb>` on the CLI,
a flat `group_verb` MCP tool name (e.g. `figma_find`) for agents.

| Group | Command | Description | Uses AI |
|---|---|---|:---:|
| — | `figma-map doctor` | Check bridge, Chrome, Storybook, and API key | — |
| **figma** (read Figma ground truth) | `figma find <query>` | Search nodes by name/text/type | — |
| | `figma inspect <nodeId>` | Node subtree: structure, text, bounds, optional `--tokens` | — |
| | `figma selection` | Get the node(s) currently selected in the editor | — |
| | `figma pages` | List the file's pages — discovery entry point | — |
| | `figma tokens <nodeId>` | Exact design tokens (color/spacing/font/radius) for a node | — |
| | `figma variables` | The file's full Variable catalog (not per-node bindings) | — |
| **capture** (images) | `capture screenshot <nodeId>` | Render a node to PNG | — |
| | `capture render <nodeId>` | Screenshot figma-map's own raw codegen output | — |
| | `capture export <nodeId>` | Export a node to SVG/PNG/JPG | — |
| **build** (code) | `build codegen <nodeId>` | Full TSX for a frame (layout, text, UIKit components) | — |
| | `build map <nodeId>` | Identify a node's component + props → JSX | ✓ cheap |
| | `build plan <nodeId>` | Map every instance in a frame → buildable spec | ✓ cheap |
| **verify** (compare) | `verify pixeldiff <nodeId>` | Pixel-level screenshot comparison + per-region breakdown | — |
| | `verify reconcile <nodeId>` | Diff rendered output vs the design (deterministic) | — / opt-in |
| **setup** (bootstrap) | `setup scan` | Screenshot Storybook stories → `catalog/` | — |
| | `setup bind` | Match Figma sections to the catalog + infer prop schemas | ✓ once |
| | `setup components` | List the components in a binding | — |
| — | `figma-map mcp` | Run as an MCP server over stdio (for agents) | — |

Pass `--file <fileKey>` to any command when multiple Figma files are connected,
and `--json` for machine-readable output. Run `figma-map <group> <command> --help`
for full flags.

## Agent / MCP integration

figma-map is built to be driven by an AI coding agent — point it at a Figma frame
and have it build the page, verifying against the design in a loop. Every command
above is also an **MCP tool** (same names, same parameters): the CLI and the MCP
server are generated from one registry, so they never drift.

Configure your agent (Claude Code, Cursor, …):

```json
{ "mcpServers": { "figma-map": { "command": "figma-map", "args": ["mcp"] } } }
```

### The loop: build a page from a mockup

The agent owns the loop; figma-map is a deterministic tool — it measures, it
doesn't guess (see [ADR-0001](docs/adr/ADR-0001-dumb-tool.md)).

1. **`build plan <nodeId>`** → a buildable spec: layout, each component instance
   mapped to your code (import + props), exact tokens, and an honest list of
   what couldn't be mapped.
2. The agent **writes the code**, stamping each element with
   `data-figma-node="<id>"` so it can be measured later. Unmapped pieces are
   hand-built from `figma tokens`; assets come from `capture export` (not
   regenerated).
3. The agent **renders** it (a Storybook story or a dev-server URL).
4. **`verify reconcile <nodeId> --story <id>`** (or `--url`) → figma-map renders the
   implementation, reads its DOM computed styles, and diffs them against the
   design's exact tokens, returning **per-element is/should numbers**:

   ```jsonc
   { "match": false, "remaining": 2, "byElement": [
       { "nodeId": "55:1140", "name": "CTA", "diffs": [
           { "prop": "background-color", "is": "rgb(31,41,55)", "should": "#18181b" },
           { "prop": "padding-left", "is": "12px", "should": "16px" } ] } ] }
   ```
5. The agent fixes the exact properties and loops from step 3 until
   `match: true`. Add `--semantic` for an LLM check of missing elements / wrong
   assets that numbers can't catch.

Because the feedback is exact numbers tied to specific elements, the loop
converges — this is what makes an otherwise-unreliable agent reliable.

A ready-made agent skill ships at
[`.claude/skills/figma-map/SKILL.md`](.claude/skills/figma-map/SKILL.md): it
teaches an agent the loop, the `data-figma-node` contract, and when to use each
operation. Claude Code picks it up automatically when figma-map work comes up.

## Configuration

See [`figma-map.example.yaml`](figma-map.example.yaml). The API key is **never**
stored in the file — it is read from the environment variable named by
`llm.apiKeyEnv` (default `OPENAI_API_KEY`).

```yaml
bridge: http://localhost:1994
storybook: http://localhost:6007
fileKey: ""            # default file; empty = sole connected file
llm:
  baseURL: ""          # empty = OpenAI; or a gateway / Ollama endpoint
  model: gpt-4o-mini
  apiKeyEnv: OPENAI_API_KEY
figma:
  source: bridge        # "bridge" (default) or "rest" — see Limitations
  tokenEnv: FIGMA_TOKEN  # only used when source: rest
```

## Architecture

```text
cmd/                 cobra root + `figma-map mcp`
internal/
  op/                operation registry — one declaration → CLI command + MCP tool
  clibind/           binds an input struct to cobra flags/args (same tags as MCP)
  service/           all logic (deterministic-first; lazy LLM)
  config/            figma-map.yaml + env override
  figma/             Source interface + bridge/REST backends; node tokens (Style)
  storybook/         index.json → catalog; chromedp screenshots; import parsing
  render/            chromedp DOM extraction (computed styles) + screenshots
  matcher/           Matcher interface + vision implementation; ground-truth name match
  binding/           figma-map.binding.yaml model (load/save)
  codegen/           binding + props → JSX
  llm/               OpenAI-compatible vision client (configurable base URL)
backend/             leader/election relay + persistent data plane (:1994) —
                     /rpc for CLI/MCP, /issues + /compare-session for the
                     extension, persisted to ~/.figma-map/backend/*.json
bridge/
  plugin/            sandboxed JS inside Figma — node/style/variable serialization
  extension/         browser extension — flags live-page issues, links them to a Figma node
```

Each operation is declared once in `internal/op`; the CLI subcommand and the MCP
tool are both generated from it, so they cannot drift (enforced by a convergence
test). The `figma.Source` and `matcher.Matcher` interfaces are extension seams:
a Figma REST backend (`figma.source: rest`, for headless/CI) already ships
behind `figma.Source` with zero changes to any caller; an embedding-based
retriever for large libraries could be added the same way behind
`matcher.Matcher`. Layer boundaries and what
each is/isn't responsible for are fixed in
[ADR-0002](docs/adr/ADR-0002-layer-boundaries.md).

### Request flow

Nothing talks to Figma directly — every read/write goes through the backend, which
relays it over a WebSocket to a plugin running inside the open Figma file. The
backend also fronts a second, unrelated contract for the browser extension —
flagging an issue on a live page never touches the RPC/WebSocket path at all:

```mermaid
flowchart LR
    CLI["figma-map CLI"] --> SVC
    MCP["figma-map mcp (stdio)"] --> SVC
    SVC["internal/service"] -->|HTTP POST /rpc| Backend
    Backend["backend\n(:1994 — HTTP + WebSocket)"] <-->|WebSocket| Plugin
    Plugin["bridge/plugin\n(sandboxed JS inside Figma)"] --> Doc[("the open Figma file")]
    Ext["bridge/extension\n(content script on the live page)"] -->|HTTP /issues| Backend
```

`capture issues` / `capture ack` (CLI/MCP) read that same inbox — a human flags
a mismatch in the browser, the agent picks it up as structured ground truth
(screenshot, bounds, linked Figma node id), never a raw pixel guess.

The backend's per-request timeout is 30s — fine for a single node, not for fully
resolving styles/variables across a whole document. So the plugin offers two
shapes of fetch, and each `internal/service` operation picks the cheap one
whenever it can:

```mermaid
flowchart TD
    Req["a read request"] --> Need{"needs structure/name/text\nonly (find, selection),\nor everything (codegen, tokens)?"}
    Need -->|"structure only"| Lean["find_nodes / get_*(depth)\nsync name-type-text predicate;\nonly matches pay for variantModes"]
    Need -->|"full styles"| Full["get_node / get_document\nserializeStyles + resolveBoundVariables\nfor every node, memoized per walk\n(VariableCache) so repeated components\n(avatar groups, lists) resolve a shared\nvariable once, not once per occurrence"]
    Lean --> Out["fast, even on a huge file"]
    Full --> Out2["correct, bounded by --depth\nwhere the caller doesn't need\nthe whole subtree"]
```

`get_main_component_name` and dev-resources (`getDevResourcesAsync`, Dev-Mode-
only) follow the same rule: a separate, narrow call paid for only by the one
node that actually needs it, never folded into the bulk walk every other
operation shares.

### Ground-truth before vision

`bind`/`map`/`plan` only call the vision model once every cheaper, deterministic
signal Figma already gives has been tried and failed:

```mermaid
flowchart TD
    A["Which catalog component is this INSTANCE?"] --> B{"its own layer name\nmatches a component\n(free)?"}
    B -->|yes| Z1["identified — score 1.0,\nno screenshot, no LLM"]
    B -->|no| C{"its main-component name\n(one extra RPC) matches?"}
    C -->|yes| Z1
    C -->|no| D["screenshot + vision model\n(matcher.Vision)"]
    D --> Z2["best-scoring match, or NO MATCH"]

    Z1 --> E["What are its prop values?"]
    E --> F{"componentProps resolves\nby name/value (binding's\nFigmaProperty/ValueMap\noverride, else normalization)?"}
    F -->|yes| Z3["resolved — no LLM"]
    F -->|no| G["vision reads only the\nleftover props off the screenshot"]
```

The same principle extends to raw values: a fill bound to a Figma Variable with
a designer-set WEB `codeSyntax` (e.g. `--color-brand-primary`) renders as
`var(--color-brand-primary)` instead of a literal hex — ground truth from the
design file, not a guess at the project's token names.

## Limitations

Honest gaps in the current release, not hidden behaviour:

- **The binding is an AI draft.** `bind` infers prop values from story names
  using library conventions; it can miss an exact code value or invent a prop.
  **Review the binding** — that human-in-the-loop step is the design, not a bug.
- **Boolean props** are stringified (`disabled: ["false", "true"]`) and rendered
  as `disabled="true"` rather than the idiomatic bare `disabled`. Planned.
- **Import paths** come from the story source as written; relative imports stay
  relative. Adjust in the binding or normalize to your alias.
- **Static screenshots only** — hover/focus/active states are not observable, so
  variants differing only by interaction state cannot be distinguished.
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

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for the dev
workflow, and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community guidelines.

```bash
make build    # build the binary
make test     # run tests with the race detector
make lint     # golangci-lint
```

## License

[MIT](LICENSE) © Kirill Baranov

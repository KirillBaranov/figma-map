# Design: Agent Integration for figma-map

> Status: **draft for discussion** · Author: design session · Date: 2026-06-13
>
> Goal: let an AI coding agent (Claude Code, Cursor, …) drive figma-map — request
> design data, get component mappings, and **reconcile the visual** of what it
> built against the Figma design, iterating until they match.

## TL;DR (the recommendation)

1. **Build once, expose twice.** Extract a single internal `service` layer that
   holds all logic. The CLI and the MCP server become *thin* wrappers over it —
   no duplicated logic, no drift.
2. **Ship both surfaces, layered:**
   - **CLI with `--json`** + a few new subcommands → works with *any* agent that
     can run a shell, immediately, no setup.
   - **`figma-map mcp`** subcommand → an MCP server over stdio (official Go SDK
     v1.5.0) with typed tools and **image content blocks** → rich integration
     for MCP-capable agents.
3. **The headline feature is `reconcile`:** vision-LLM semantic diff between the
   Figma design and the agent's rendered output, returning *actionable
   structured feedback* the agent can loop on. This is "сводить визуал".
4. **Don't reinvent raw Figma access.** `figma-bridge` already exposes raw
   Figma over MCP. figma-map's value is the *mapping / planning / reconciliation*
   layer on top. figma-map's MCP server is the single surface the agent
   configures; it uses the bridge internally.

---

## 1. The problem & the agent loop

An agent told *"implement this Figma screen in our codebase"* runs this loop:

```
  ┌─────────────────────────────────────────────────────────────┐
  │  1. DISCOVER   what's in the design (structure, text, comps)  │  figma-map
  │  2. PLAN       which code components + props to use           │  figma-map
  │  3. GENERATE   write the code                                 │  the agent
  │  4. RENDER     build it, screenshot it                        │  agent (+ figma-map)
  │  5. RECONCILE  compare render vs design → feedback            │  figma-map  ◄── new
  │  6. ITERATE    fix code, back to 4                            │  the agent
  └─────────────────────────────────────────────────────────────┘
```

figma-map owns **1, 2, 5** (and helps with 4). Steps 3 and 6 are the agent's.

The user's phrasing — *"чтобы агент мог запрашивать нужные данные и сводить визуал
с этим"* — maps to two needs:

- **A. Data access** — query the design + the mapping (steps 1, 2).
- **B. Visual reconciliation** — compare implementation vs design (step 5).

## 2. Where figma-map sits vs figma-bridge

This is the key architectural boundary.

| Concern | Owner |
|---|---|
| Raw Figma access (node tree, screenshots, styles, edits) | **figma-bridge** (already an MCP server + `/rpc`) |
| Code catalog, component mapping, prop inference, codegen, **visual reconciliation** | **figma-map** (the value layer) |

So figma-map's MCP tools should **not** duplicate `get_node` / `get_screenshot`.
Instead figma-map exposes the value layer and a *thin, enriched* pass-through for
inspect/screenshot — so an agent configures **only figma-map** and gets
everything, with the bridge running invisibly underneath. Power users who want
raw Figma *editing* can additionally connect the bridge MCP directly.

## 3. Two integration surfaces

### 3a. CLI (`--json`)

Agents already run shells. figma-map is already a CLI. Add machine-readable
output and a couple of commands.

- **Pros:** universal (any agent), zero protocol/setup, scriptable, trivially
  testable, composes with unix tools.
- **Cons:** agent constructs commands as text (more error-prone than typed
  tools); images returned as file paths (fine for Claude Code, which reads
  images; awkward for some agents); no schema discovery.

### 3b. MCP server (`figma-map mcp`)

Expose typed tools over stdio using the official Go SDK.

- **Pros:** typed, discoverable tools agents call reliably; **native image
  content** (perfect for reconcile — image in, image+feedback out); a stateful
  server can cache catalog/binding and hold the bridge connection; the standard
  way agents integrate in 2026.
- **Cons:** more to build/maintain; agent must configure the MCP server.

### Verdict: do both, layered

Because the logic lives in a shared `service` layer, the MCP server is thin and
the CLI is essentially free. CLI is the universal fallback; MCP is the
first-class experience. **No either/or.**

```
                 ┌──────────────────────────┐
   agent ──CLI──▶│  cmd/* (cobra, --json)    │─┐
                 └──────────────────────────┘ │   ┌───────────────────┐
                 ┌──────────────────────────┐ ├──▶│ internal/service  │──▶ figma / storybook
   agent ──MCP──▶│  cmd/mcp.go (Go SDK)      │─┘   │ (all the logic)   │    matcher / codegen / llm
                 └──────────────────────────┘     └───────────────────┘
```

## 4. Capability catalog (agent-facing)

Each capability is one `service` method, surfaced as a CLI subcommand (`--json`)
**and** an MCP tool. `*` marks new work.

| Capability | What the agent gets | AI | CLI | MCP tool |
|---|---|:--:|---|---|
| **doctor** | env readiness (bridge/chrome/storybook/key) | — | `doctor --json` | `figma_doctor` |
| **list** | bound components: name, import, props | — | `list --json` | `figma_list_components` |
| **inspect** \* | node tree: type, name, text, bbox, styles; with `--annotate`, each instance tagged with its mapped component | opt | `inspect <id> [--annotate]` | `figma_inspect` |
| **screenshot** \* | rendered image of a node | — | `screenshot <id> -o f.png` | `figma_screenshot` → ImageContent |
| **map** | one node → component + props + import + JSX | ✓ | `map <id> --json` | `figma_map_component` |
| **plan** \* | a frame → ordered, buildable spec of all component instances + layout + unmapped leftovers | ✓ | `plan <frameId> --json` | `figma_plan` |
| **reconcile** \* | design vs rendered → structured, actionable diff | ✓ | `reconcile <id> --image f.png \| --story id` | `figma_reconcile` |

### 4a. `plan` — the design→code blueprint

The bridge between "raw design" and "code the agent can write". Walk a frame,
find component instances, `map` each, return a buildable spec:

```json
{
  "frame": { "id": "55:1102", "name": "Login", "width": 396, "height": 620 },
  "layout": { "mode": "vertical", "gap": 16, "padding": [32,32,64,32] },
  "components": [
    { "nodeId": "55:1110", "component": "Input", "symbol": "Input",
      "import": "@/components/ui/input", "props": {}, "text": "",
      "bbox": {"x":32,"y":120,"w":332,"h":40}, "confidence": 0.81 },
    { "nodeId": "55:1140", "component": "Button", "symbol": "Button",
      "import": "@/components/ui/button", "props": {"variant":"default"},
      "text": "Continue", "bbox": {"x":32,"y":180,"w":332,"h":40},
      "confidence": 0.86 }
  ],
  "unmapped": [
    { "nodeId": "55:1150", "name": "Divider", "type": "LINE",
      "reason": "no catalog match above threshold" }
  ]
}
```

The agent now has: the layout container, the ordered list of components with
imports + props + text, and an honest list of what figma-map *couldn't* map (so
it doesn't silently drop UI).

### 4b. `reconcile` — visual reconciliation (the heart)

Given the Figma node and the agent's rendered output, return **what differs and
how to fix it**. We deliberately use a **vision-LLM semantic diff**, not pixel
diff, because the agent needs *actionable* feedback that survives minor
alignment/aspect differences.

Inputs (one of):
- `--image rendered.png` — arbitrary render the agent produced, **or**
- `--story <storyId>` — figma-map screenshots the agent's Storybook story itself
  (reuses the existing `storybook.Capturer`), so the agent just names a story.

Output:

```json
{
  "match": false,
  "similarity": 0.82,
  "summary": "Structure matches; spacing and one label differ.",
  "differences": [
    { "kind": "text", "severity": "major",
      "detail": "Button reads 'Submit'; design says 'Continue'.",
      "suggestion": "Change the button label to 'Continue'." },
    { "kind": "spacing", "severity": "minor",
      "detail": "Gap between field and button ~8px; design ~16px.",
      "suggestion": "Increase the vertical gap to 16px." },
    { "kind": "missing", "severity": "major",
      "detail": "Left icon inside the button is absent." }
  ]
}
```

`kind` ∈ {layout, spacing, size, color, typography, text, missing, extra,
alignment, border, other}; `severity` ∈ {major, minor, cosmetic}. The MCP
variant can *also* return an annotated side-by-side image (ImageContent) for the
agent to look at directly.

This closes the loop: the agent edits code from `suggestion`s, re-renders,
re-`reconcile`s until `match: true` (or only `cosmetic` left).

## 5. Architecture & implementation

### 5a. Shared service layer (the refactor)

Today each `cmd/*.go` wires config → bridge → catalog → matcher inline. Extract
that into `internal/service`:

```go
package service

type Service struct {
    cfg     config.Config
    src     figma.Source
    matcher matcher.Matcher
    // lazily loaded + cached:
    catalog *storybook.Catalog
    binding *binding.Binding
}

func New(cfg config.Config) (*Service, error)

func (s *Service) Doctor(ctx) (Report, error)
func (s *Service) ListComponents() ([]ComponentInfo, error)
func (s *Service) Inspect(ctx, nodeID string, annotate bool) (InspectResult, error)
func (s *Service) Screenshot(ctx, nodeID string) ([]byte, error)
func (s *Service) MapNode(ctx, nodeID string) (MapResult, error)
func (s *Service) Plan(ctx, frameID string) (Plan, error)
func (s *Service) Reconcile(ctx, nodeID string, rendered []byte) (Diff, error)
```

`cmd/*` and `cmd/mcp.go` both call these. All results are plain structs that
serialize to the JSON shown above — the CLI prints them, the MCP server returns
them as structured content.

### 5b. The `figma-map mcp` subcommand

Single binary, new subcommand starts an stdio MCP server:

```go
// cmd/mcp.go (sketch, official SDK)
srv := mcp.NewServer(&mcp.Implementation{Name: "figma-map", Version: version}, nil)
mcp.AddTool(srv, &mcp.Tool{Name: "figma_plan",
    Description: "Map every component instance in a Figma frame to code."},
    func(ctx, req, in PlanIn) (*mcp.CallToolResult, PlanOut, error) {
        p, err := svc.Plan(ctx, in.FrameID)
        // return structured + (for screenshot/reconcile) ImageContent
    })
// ... register the other tools ...
srv.Run(ctx, &mcp.StdioTransport{})
```

Agent config (Claude Code / Cursor):

```json
{ "mcpServers": { "figma-map": { "command": "figma-map", "args": ["mcp"] } } }
```

### 5c. SDK choice

Use the **official `github.com/modelcontextprotocol/go-sdk`** (v1.5.0, stable,
maintained with Google, supports `ImageContent` in `CallToolResult`). Rationale:
forward-looking, first-party, image support is exactly what reconcile/screenshot
need. (Alternative: `mark3labs/mcp-go` — mature community SDK; keep as fallback.)

## 6. Risks & mitigations

| Risk | Mitigation |
|---|---|
| **Reconcile loops burn image tokens** | `detail:low`; cache the Figma side per node (it doesn't change between iterations); only the rendered side is re-sent. |
| **Vision diff nitpicks / false "match"** | Constrain `kind`s and ask only for *actionable* differences; surface `severity` so the agent ignores cosmetic noise; report `similarity` not just boolean. |
| **Aspect/alignment mismatch render vs design** | Vision diff is robust to it (pixel diff is not) — that's why we chose it. Offer optional pixel-diff later for pixel-perfect teams. |
| **Stale binding/catalog in long-lived MCP server** | A `figma_refresh` tool / mtime check; `doctor` reports staleness. |
| **Agent calls `plan` on a huge frame** | Cap instance count, dedupe identical instances, return `unmapped` honestly; clear error over a silent truncation. |
| **Bridge requires Figma desktop open** | `doctor` tool surfaces it; every tool returns a clear, fixable error if the bridge/file is absent. |
| **Confidential designs sent to a cloud LLM** | `llm.baseURL` already supports local models (Ollama/llava); document the privacy posture in SECURITY.md. |

## 7. Phased plan

- **Phase 0 — Refactor + JSON (low risk, unlocks CLI agents now).**
  Extract `internal/service`; add `--json` to existing commands. Agents can use
  figma-map via shell immediately.
- **Phase 1 — New value commands.** `inspect` (+`--annotate`), `plan`,
  `reconcile`. These are the data-access + reconciliation the user asked for.
- **Phase 2 — MCP server.** `figma-map mcp` over the official SDK, wrapping the
  same service, with image content for `screenshot`/`reconcile`.
- **Phase 3 — Polish.** Annotated side-by-side overlay images, result caching,
  a shipped Claude Code skill (`.claude/skills/figma-map.md`), and docs/examples.

## 8. Open questions for discussion

1. **Reconcile input:** support both `--image` and `--story`? (Recommend yes —
   `--story` is ergonomic, `--image` is universal.)
2. **`plan` recursion:** one level of instances, or recurse nested frames?
   (Recommend one level + a `--depth` flag later.)
3. **Single surface vs two servers:** confirm figma-map MCP should be the *only*
   server the agent configures (bridge hidden underneath). (Recommend yes.)
4. **Pixel-diff option** alongside vision diff for pixel-perfect teams? (Later.)
5. **Reconcile model:** allow a *different* (cheaper/local) model than matching?
6. **Output verbosity:** does the agent want JSX in `plan` per component, or just
   the spec and let it write JSX itself? (Lean: spec + import + props; JSX
   on-demand via `map`.)
```

# Commands & configuration reference

> Full CLI/MCP surface and config file. See the [README](../README.md) for
> install and a 5-minute quick start.

## Commands

Commands are grouped by what they do — `figma-map <group> <verb>` on the CLI,
a flat `group_verb` MCP tool name (e.g. `figma_find`) for agents.

| Group | Command | Description | Uses AI |
|---|---|---|:---:|
| — | `figma-map doctor` | Check bridge, Chrome, Storybook, and API key | — |
| **bridge** (local backend process) | `bridge up [--repo]` | Start the backend if nothing's listening yet (builds it first if needed) | — |
| | `bridge status` | Check whether the backend is reachable, and its pid/log | — |
| | `bridge down` | Stop the backend `bridge up` started | — |
| **figma** (read Figma ground truth) | `figma find <query>` | Search nodes by name/text/type | — |
| | `figma inspect <nodeId>` | Node subtree: structure, text, bounds, optional `--tokens` | — |
| | `figma selection` | Get the node(s) currently selected in the editor | — |
| | `figma pages` | List the file's pages — discovery entry point | — |
| | `figma tokens <nodeId>` | Exact design tokens (color/spacing/font/radius) for a node | — |
| | `figma animation <nodeId>` | Resolve a node's reactions to actual before/after style deltas | — |
| | `figma variables` | The file's full Variable catalog (not per-node bindings) | — |
| **capture** (images) | `capture screenshot <nodeId>` | Render a node to PNG | — |
| | `capture render <nodeId>` | Screenshot figma-map's own raw codegen output | — |
| | `capture browser <url> [--selector]` | Screenshot a live URL, optionally cropped to one element | — |
| | `capture export <nodeId>` | Export a node to SVG/PNG/JPG | — |
| **build** (code) | `build codegen <nodeId>` | Full TSX for a frame (layout, text, UIKit components) | — |
| | `build map <nodeId>` | Identify a node's component + props → JSX | ✓ cheap |
| | `build plan <nodeId>` | Map every instance in a frame → buildable spec | ✓ cheap |
| **verify** (compare) | `verify pixeldiff <nodeId> [--selector]` | Pixel-level screenshot comparison + per-region breakdown | — |
| | `verify reconcile <nodeId>` | Diff rendered output vs the design (deterministic) | — / opt-in |
| **setup** (bootstrap) | `setup scan` | Screenshot Storybook stories → `catalog/` | — |
| | `setup bind` | Match Figma sections to the catalog + infer prop schemas | ✓ once |
| | `setup components` | List the components in a binding | — |
| — | `figma-map mcp` | Run as an MCP server over stdio (for agents) | — |
| — | `figma-map init [path]` | Scaffold the skill, config, and MCP registration into a project | — |

Pass `--file <fileKey>` to any command when multiple Figma files are connected,
and `--json` for machine-readable output. Run `figma-map <group> <command> --help`
for full flags.

## MCP integration

Every command in the table above except `mcp` and `init` itself is also an
**MCP tool** (same names, same parameters): the CLI and the MCP server are
generated from one shared registry, so they never drift. This is what lets
an agent drive the whole loop itself, tool call by tool call.

`figma-map init <path>` writes this registration into the target project's
`.mcp.json` for you (merged in, not overwriting any other servers already
configured there). To configure it by hand instead — or for an agent whose
config lives somewhere other than `.mcp.json` (Claude Code, Cursor, …):

```json
{ "mcpServers": { "figma-map": { "command": "figma-map", "args": ["mcp"] } } }
```

## Configuration

See [`figma-map.example.yaml`](../figma-map.example.yaml). The API key is **never**
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
  source: bridge        # "bridge" (default) or "rest" — see docs/limitations.md
  tokenEnv: FIGMA_TOKEN  # only used when source: rest
```

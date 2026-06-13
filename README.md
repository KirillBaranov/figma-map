# figma-map

Map Figma design components to your code component library using a vision LLM.

figma-map answers the question *"which `<Button>` is this Figma layer, and with
what props?"* — automatically. Instead of manually maintaining a mapping between
your design system in Figma and your component library in code, figma-map builds
that mapping once with AI, lets you review it, then applies it deterministically.

## How it works: bind → apply

The tool is built around a reviewable artifact, `figma-map.binding.yaml`, the
same way codegen and i18n tools work:

1. **scan** — screenshot every Storybook story → a code-component catalog.
2. **bind** *(AI, once)* — match each Figma component section to the catalog and
   infer each component's prop schema → write `figma-map.binding.yaml`.
3. *review the binding* — it's a draft; fix anything the model got wrong.
4. **map** *(cheap, repeatable)* — for any Figma node, identify its component and
   prop values from the binding and emit JSX.

The AI runs during `bind`. Once the binding is reviewed and committed, mapping is
reproducible and CI-friendly.

## Requirements

- **Go 1.24+** to build.
- **Google Chrome / Chromium** installed (used headless for Storybook
  screenshots).
- A running **Storybook** with an `index.json` endpoint (Storybook 7+).
- A running **[figma-mcp-bridge](https://github.com/gethopp/figma-mcp-bridge)**:
  its plugin connects an open Figma file to a local HTTP server on `:1994`,
  bypassing Figma REST API rate limits.
- An **OpenAI-compatible** vision endpoint + API key. Works with OpenAI, the
  kb-labs gateway, or a local Ollama/llava server via `llm.baseURL`.

## Install

One line — detects your OS/arch, downloads the matching release, verifies its
SHA-256 checksum, and installs the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/kirillbaranov/figma-map/main/install.sh | sh
```

Overrides: `FIGMA_MAP_VERSION=v0.1.0` to pin a tag, `FIGMA_MAP_INSTALL_DIR=~/bin`
to choose the directory.

Or with Go:

```bash
go install github.com/kirillbaranov/figma-map@latest
```

Or grab a prebuilt archive from the [releases page](https://github.com/kirillbaranov/figma-map/releases).

## Setup

```bash
cp figma-map.example.yaml figma-map.yaml   # edit as needed
export OPENAI_API_KEY=sk-...

figma-map doctor                           # verify bridge, chrome, storybook, key
```

## Usage

```bash
# 1. Build the code-component catalog (no AI).
#    --project points at the repo containing your *.stories.tsx files,
#    used to resolve each component's real import path.
./figma-map scan --project /path/to/your/storybook-project

# 2. Match Figma to the catalog and write the binding (AI, run once).
./figma-map bind                           # uses the sole connected Figma file
#   review figma-map.binding.yaml here

# 3. Generate code for any Figma node.
./figma-map map 13:1077
# // 13:1077 → Button (0.80)
# import { Button } from "@/components/ui/button"
#
# <Button>View docs</Button>
```

Pass `--file <fileKey>` to any command when multiple Figma files are connected.

## Configuration

See [`figma-map.example.yaml`](figma-map.example.yaml). The API key is never
stored in the file — it is read from the environment variable named by
`llm.apiKeyEnv`.

## Architecture

```
cmd/                 cobra subcommands (doctor, scan, bind, map)
internal/
  config/            figma-map.yaml + env override
  figma/             Source interface + bridge (/rpc) backend
  storybook/         index.json → catalog; chromedp screenshots; import parsing
  matcher/           Matcher interface + vision implementation
  binding/           figma-map.binding.yaml model (load/save)
  codegen/           binding + props → JSX
  llm/               OpenAI-compatible vision client (configurable base URL)
```

The `figma.Source` and `matcher.Matcher` interfaces are extension seams: a Figma
REST backend (for CI, where no desktop bridge runs) and an embedding-based
retriever (for large libraries) can be added without touching callers.

## Known limitations

These are honest gaps in the current vertical slice, not hidden behaviour:

- **Prop schema is an AI draft.** `bind` infers prop values from story names
  using library conventions; it can miss the exact code value (e.g. shadcn's
  `default` vs an inferred `primary`/`md`) or invent a prop. **Review the
  binding.** This is by design — the binding is the human-in-the-loop artifact.
- **Boolean props** are stringified (`disabled: ["false","true"]`) and rendered
  as `disabled="true"` rather than the idiomatic bare `disabled`. Planned.
- **Import paths** come from the story source as written; relative imports stay
  relative. Adjust in the binding or normalize to your alias.
- **Static screenshots only** — hover/focus/active states aren't observable, so
  variants that differ only by interaction state can't be distinguished.
- **The bridge requires Figma desktop open** with the plugin running. A REST
  backend for headless/CI use is a planned `figma.Source` implementation.

## License

MIT

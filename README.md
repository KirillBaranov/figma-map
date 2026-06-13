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

## Contents

- [Why](#why)
- [How it works: bind → apply](#how-it-works-bind--apply)
- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
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
| **[figma-mcp-bridge](https://github.com/gethopp/figma-mcp-bridge)** running | connects an open Figma file to a local server on `:1994`, bypassing Figma API rate limits |
| **OpenAI-compatible vision endpoint + key** | matching and prop inference (works with OpenAI, a local Ollama/llava server, or any compatible gateway via `llm.baseURL`) |

## Quick start

```bash
cp figma-map.example.yaml figma-map.yaml      # adjust URLs if needed
export OPENAI_API_KEY=sk-...

figma-map doctor                              # verify bridge, chrome, storybook, key

# 1. Build the code-component catalog (no AI).
#    --project points at the repo containing your *.stories.tsx files.
figma-map scan --project /path/to/storybook-project

# 2. Match Figma to the catalog and write the binding (AI, run once).
figma-map bind
#    → review figma-map.binding.yaml

# 3. Generate code for any Figma node.
figma-map map 13:1077
```

## Commands

| Command | Description | Uses AI |
|---|---|:---:|
| `figma-map doctor` | Check bridge, Chrome, Storybook, and API key | — |
| `figma-map scan` | Screenshot Storybook stories → `catalog/` | — |
| `figma-map bind` | Match Figma sections to the catalog + infer prop schemas → `figma-map.binding.yaml` | ✓ once |
| `figma-map map <nodeId>` | Identify a node's component + props → JSX | ✓ cheap |

Pass `--file <fileKey>` to any command when multiple Figma files are connected.
Run `figma-map <command> --help` for full flags.

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
```

## Architecture

```text
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

The `figma.Source` and `matcher.Matcher` interfaces are deliberate extension
seams. A Figma REST backend (for CI, where no desktop bridge runs) and an
embedding-based retriever (for large libraries) can be added without touching
callers.

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
- **The bridge requires Figma desktop open** with the plugin running. A REST
  backend for headless/CI use is a planned `figma.Source` implementation.

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

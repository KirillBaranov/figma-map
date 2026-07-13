<div align="center">

# figma-map

**The ground-truth layer that lets AI coding agents build pixel-perfect UI from Figma.**

Agents that build from a screenshot guess. figma-map gives them exact
structure and tokens to build from, and a closed verify loop — render the
implementation, diff its real DOM against the design's exact values — so the
agent knows precisely what's still wrong instead of eyeballing "looks about
right."

[![CI](https://github.com/KirillBaranov/figma-map/actions/workflows/ci.yml/badge.svg)](https://github.com/KirillBaranov/figma-map/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kirillbaranov/figma-map.svg)](https://pkg.go.dev/github.com/kirillbaranov/figma-map)
[![Go Report Card](https://goreportcard.com/badge/github.com/kirillbaranov/figma-map)](https://goreportcard.com/report/github.com/kirillbaranov/figma-map)
[![Release](https://img.shields.io/github/v/release/KirillBaranov/figma-map?sort=semver)](https://github.com/KirillBaranov/figma-map/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

---

![design vs an agent building by eye vs an agent using figma-map](bench/sidebyside.png)

<p align="center"><em>Left to right: the design, an agent building by eye, an agent using figma-map's tokens + verify loop. Same model, same prompt, same assets — the only variable is the tool.</em></p>

<div align="center">

| Case | Design | by eye | figma-map | Independent pixel diff |
|---|---|---|---|---|
| [Landing hero](bench/REPORT.md) | decorative, photo + gradient | 7.83% | 4.10% | **~48% closer** |
| [Landing hero 2](bench/cases/landing-hero-2/REPORT.md) | dark hero, feature cards | 8.17% | 5.98% | **~27% closer** |
| [Admin dashboard](bench/cases/admin-dashboard/REPORT.md) | real component-dense UI (sidebar, cards, charts, list) | 8.87% | 1.92% | **~78% closer** |

</div>

<p align="center">Same agent, same model, same prompt, same shared assets — only the tool differs. <strong>By eye:</strong> the agent gets a screenshot and "build this." <strong>figma-map:</strong> the agent gets the Figma selection through the bridge, builds from figma-map's tokens/plan, then renders and compares its own output with figma-map's <code>reconcile</code> (plus its own browser MCP tools) until the diff closes. The score itself is not figma-map's — a plain, independent pixel diff against the reference image, so it can't be biased toward the treatment arm. <a href="bench/README.md">Method & caveats →</a></p>

## What it is

```jsonc
// verify reconcile 55:1140 --story cta-banner
{ "match": false, "remaining": 2, "byElement": [
    { "nodeId": "55:1140", "name": "CTA", "diffs": [
        { "prop": "background-color", "is": "rgb(31,41,55)", "should": "#18181b" },
        { "prop": "padding-left", "is": "12px", "should": "16px" } ] } ] }
```

That's the primitive everything else is built on: not "does this look
right," but *exactly* which element, which property, which value — an agent
can act on that and loop until it's gone.

- **A closed verify loop, not a screenshot to eyeball.** figma-map renders your
  implementation, reads its actual DOM, and diffs computed styles against the
  design's exact tokens — per-element `is → should` numbers the agent can fix,
  not vibes to interpret.
- **Ground truth before vision, everywhere.** Structure, tokens, and component
  identity are read straight from Figma's data model whenever it has the
  answer. A vision model only steps in for the one question Figma's data can't
  answer — *which code component is this?* — never as the default.
- **AI runs once, codegen runs forever.** That one vision-dependent question is
  answered once, into a reviewable binding file. Every generation and
  verification pass after that is deterministic, repeatable, CI-friendly code —
  no LLM in the hot path.
- **MCP-native.** Point an agent (Claude Code, Cursor, …) at it and it gets the
  full CLI as typed tools, identical surface, zero drift.
- **Closes the loop on live pages too.** A browser extension lets a human flag
  a mismatch on a running page and link it straight to its Figma node — the
  agent picks that up as structured ground truth, never a raw pixel guess.

<div align="center">
<img src="docs/screenshots/browser-plugin-select-issue.png" width="49%" alt="flagging a live-page mismatch and linking it to a Figma node" />
<img src="docs/screenshots/browser-plugin-diff-mode.png" width="49%" alt="overlay diff mode comparing the live page against the design" />
</div>

<p align="center"><em>A human clicks the element that's off, notes what's wrong, and sends it — the agent gets a Figma node id and bounds, not "the card looks weird."</em></p>

## Why

Pointing an agent at a Figma screenshot and asking it to "build this" mostly
works — until it doesn't, and there's no way to tell *how far off* without a
human eyeballing a diff. The agent picked a color close enough, a padding
that's 4px short, the wrong variant of your `<Button>` — and nothing in the
loop can tell it that, so it stops when it *looks* done, not when it *is*
done.

figma-map closes that loop. It reads Figma's actual data — not a rendered
picture of it — for structure, tokens, and (via a one-time binding) component
identity, and it can re-render the agent's own output and diff it against
that same ground truth, property by property. The agent gets exact numbers to
fix, not vibes to interpret, so the loop actually converges on the design
instead of stalling on "close enough."

## How it works

1. **`build plan`** → a buildable spec for a Figma node: layout, each
   component instance mapped to your code (import + props), exact tokens.
2. The **agent writes the code**, tagging each element so it can be measured
   later.
3. The agent **renders** it (Storybook or a dev server).
4. **`verify reconcile`** renders the implementation, reads its DOM, and diffs
   it against the design's exact tokens — per-element `is`/`should` numbers.
5. The agent **fixes the exact properties** and loops from step 3 until
   everything matches.

A ready-made agent skill ships in [`.claude/skills/figma-map`](.claude/skills/figma-map/SKILL.md)
that teaches Claude Code this loop automatically. For the full request-flow
diagrams and the "ground truth before vision" decision tree, see
[docs/architecture.md](docs/architecture.md).

### Component identity, solved once

The one place vision is unavoidable is matching a Figma instance to *your*
code component — Figma's data model has no field for "this is our
`<Button>`." figma-map solves it once, up front, into a reviewable
`figma-map.binding.yaml` you can correct by hand. Every generation after that
is deterministic:

```text
Storybook ──scan──▶ catalog (screenshots + import paths, no AI)
Figma ──bind (vision LLM, once)──▶ figma-map.binding.yaml ──review──▶ map (deterministic) ──▶ JSX
```

## Install

### 1. Install the CLI

```bash
curl -fsSL https://raw.githubusercontent.com/KirillBaranov/figma-map/main/install.sh | sh
```

Detects your OS/arch, downloads the matching release, verifies its SHA-256
checksum, and installs the binary. Override with `FIGMA_MAP_VERSION=v0.1.0`
to pin a tag, or `FIGMA_MAP_INSTALL_DIR=~/bin` to choose the directory.

Alternatives: `go install github.com/kirillbaranov/figma-map@latest`, or grab
a prebuilt archive from the [releases page](https://github.com/KirillBaranov/figma-map/releases).

### 2. Load the bridge plugin in Figma

The bridge is what lets figma-map read your open Figma file directly, with no
API token and no rate limits.

```bash
figma-map bridge up --repo <path to this checkout>   # builds + starts the local backend on :1994
```

Then, **once**, load the plugin into Figma:

1. Open your Figma file (desktop app).
2. **Plugins → Development → Import plugin from manifest…**
3. Select `extensions/plugin/manifest.json` from this checkout.
4. Run it once (**Plugins → Development → Figma MAP Bridge**) — it connects
   over WebSocket to the backend you just started and stays connected while
   the file is open.

<p align="center"><img src="docs/screenshots/figma-plugin-screen.png" width="80%" alt="Figma MAP Bridge plugin panel, connected, with the ready-to-paste agent prompt" /></p>

<p align="center"><em>Connected, and it already wrote the exact prompt for your agent — file, selection, node id.</em></p>

### 3. (Optional) Load the browser extension

Lets a human flag a mismatch on a running page and hand it to the agent as a
Figma-linked issue, instead of a screenshot and a paragraph of description.

1. `cd extensions/browser && npm install && npm run build`
2. Open `chrome://extensions`, enable **Developer mode**.
3. **Load unpacked** → select `extensions/browser/dist`.

### Requirements

The only hard requirement is the Figma plugin (step 2 above) — everything
else below is optional, and just means a specific feature won't work
without it.

| Dependency | Required? | Without it |
|---|---|---|
| **Figma desktop, bridge + plugin running** (step 2 above) | **Yes** | Nothing works — this is how figma-map reads your file at all. |
| **Google Chrome / Chromium** | Optional | No headless rendering — `screenshot`, `verify reconcile`, and the browser-extension compare loop need it. |
| **Storybook 7+** running | Optional | No code-component catalog — `setup scan`/`build map` (going from a Figma node to *your* JSX) need it. Reading tokens/structure straight from Figma doesn't. |
| **OpenAI-compatible vision endpoint + key** | Optional | No component matching or prop inference — `setup bind` and the leftover-prop vision step need it (works with OpenAI, a local Ollama/llava server, or any compatible gateway). |
| **Browser extension** (step 3 above) | Optional | No human-flagged live-page issues — everything else still works without it. |

## Quick start

```bash
figma-map init /path/to/your/project          # skill, figma-map.yaml, MCP registration, CLAUDE.md
cd /path/to/your/project
export OPENAI_API_KEY=sk-...

figma-map doctor                              # verify bridge, chrome, storybook, key

# 1. Build the code-component catalog (no AI).
figma-map setup scan --project /path/to/storybook-project

# 2. Match Figma to the catalog and write the binding (AI, run once).
figma-map setup bind
#    → review figma-map.binding.yaml

# 3. Generate code for any Figma node.
figma-map build map 13:1077
```

`init` never clobbers what's already there — it prints exactly what it's
about to create/change and asks for confirmation first (`-y` to skip that for
scripts). Full command list and every flag: [docs/commands.md](docs/commands.md).

## Troubleshooting

> **Bridge disconnected? This is almost always it:** Figma freezes plugin
> execution (including the WebSocket connection) when the Figma window loses
> focus or is minimized — that's Figma's behavior, not a figma-map bug. If
> the bridge drops, you (or the agent) most likely had Figma closed, in the
> background, or minimized for a while. **Bring Figma back to the
> foreground** — the plugin reconnects on its own — and have the agent retry
> the call. No restart of the backend needed.

Other common issues:

- **`figma-map doctor` fails on "bridge"** — the backend (`:1994`) isn't
  running, or no plugin instance has ever connected to it. Run
  `figma-map bridge up --repo <path>`, then load/run the plugin in Figma once
  (see [Install → step 2](#2-load-the-bridge-plugin-in-figma)).
- **`doctor` fails on "chrome"** — no local Chrome/Chromium found; install
  one, or point `figma-map.yaml` at a binary via the chrome path setting.
- **`doctor` fails on "storybook"** — nothing is serving `index.json` on the
  configured URL; start Storybook (`npm run storybook` or equivalent) first.
- **A request stalls on a huge document** — the plugin heartbeats while it's
  still working and the backend's timeout resets on each one, so it won't
  get killed just for taking a while; but a full-document styles walk over a
  very large node count is still genuinely slow. Scope the call with
  `--depth` instead of walking the whole file (see
  [docs/architecture.md](docs/architecture.md#request-flow)).
- **Still stuck after Figma is in the foreground and reconnected** — restart
  the backend (`figma-map bridge down && figma-map bridge up --repo <path>`)
  and re-run the plugin once from **Plugins → Development**.

## Roadmap

Where this is headed next, in rough priority order:

- [ ] **Deeper agent ↔ issue integration** — a tighter, more explicit hand-off
  than "the agent polls the inbox": the agent should be able to claim an
  issue, report progress back on it, and close the loop without the human
  re-explaining context already captured when it was flagged.
- [ ] **Arbitrary-region diff selection** — today a flagged issue is tied to
  one element; letting a human drag-select any region of the page (not just
  a single node) and hand *that* to the agent as the unit of comparison.
- [ ] **Diff-to-fix, not just diff-to-look-at** — show the agent the actual
  visual diff for a flagged issue (not only the Figma-side tokens), so it can
  reason about what changed on screen, not just what the spec says.
- [ ] **Large-document performance** — the backend no longer times out a
  large request outright (the plugin heartbeats, the backend's inactivity
  window resets on each one), but a full-styles walk over a document with a
  very large node count is still genuinely slow. The workaround today is
  scoping calls with `--depth` (see [Troubleshooting](#troubleshooting));
  actually speeding up the walk itself is still open.
- [ ] **One-click plugin/extension install** — publish the Figma plugin to
  the Community and the browser extension to the Chrome Web Store, so step
  2/3 above stop being a manual "load unpacked."
- [ ] **Wider `reconcile` coverage** — margins, box-shadow, and gradient
  fills aren't diffed yet.
- [ ] **Idiomatic boolean props** in codegen (`disabled` instead of
  `disabled="true"`).
- [ ] **REST-source parity** — close the gaps between the live bridge and
  the headless/CI-friendly REST backend (bound-variable resolution,
  prototyping reactions, dev-resources, annotations).

See [docs/limitations.md](docs/limitations.md) for the full, current list of
gaps — and [CHANGELOG.md](CHANGELOG.md) for what's already shipped.

## Documentation

The above is enough to get productive. For everything else:

- [docs/architecture.md](docs/architecture.md) — folder layout, request flow,
  the ground-truth-before-vision decision tree
- [docs/commands.md](docs/commands.md) — every CLI/MCP command, flags, and
  `figma-map.yaml` config
- [docs/limitations.md](docs/limitations.md) — honest, current gaps
- [docs/adr/](docs/adr) — architecture decision records
- [CHANGELOG.md](CHANGELOG.md) — release history

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

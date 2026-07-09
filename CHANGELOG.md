# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-07-10

### Added

- **`init`** — one-command project onboarding. Scaffolds the bundled Claude
  Code skill, a starter `figma-map.yaml`, and figma-map's MCP server
  registration into a target project's `.mcp.json` (merged in, existing
  servers untouched), plus a re-runnable, delimited section in that
  project's `CLAUDE.md`. Picks the target interactively (fuzzy-filterable)
  or accepts a path for scripted/CI use; always previews what it's about to
  write and asks for confirmation first (`-y` to skip, `--force` to
  overwrite a skill file that's diverged from the bundled version).
- **Figma REST Source** — an optional read-only backend that talks to the
  Figma REST API directly, for ground-truth reads that don't need the
  bridge/plugin round-trip.
- `capture issues` / `capture ack` — an inbox of regions a human flagged via
  the browser extension (screenshot, bounds, CSS selector, optional linked
  Figma node, note), for pairing with `verify pixeldiff-images`.
- `verify pixeldiff-images` — pixel diff between two already-captured
  images directly, no Figma node lookup or browser render needed.
- Browser extension: a bottom bar with per-window state, a per-site
  allowlist (a small "+" by default, the full bar only on enabled sites),
  a hover-selector overlay showing the CSS selector and size, and an
  issue-capture history with pin/remove.
- CSS `var()` emission for Figma Variables that carry a WEB `codeSyntax`,
  instead of always inlining the literal value.
- `.env` is loaded automatically for `OPENAI_API_KEY` (and other secrets) —
  no more requiring it to be exported by hand.
- `find`/`inspect` accept a `--depth` limit, so a large subtree that used to
  time out can be fetched incrementally instead.

### Changed

- **MCP tool schemas now mark only truly-required fields as `required`.**
  Previously every field was required in the generated schema (a JSON
  Schema inference quirk), and worse, the MCP path never applied the same
  `default` tag values the CLI gets from its cobra flags — an MCP caller
  omitting an optional field like `binding`/`catalog` could hit a raw
  `open : no such file or directory` instead of the documented default.
  Both surfaces now agree.
- **Large `get_document`/`get_selection` calls stream instead of blocking
  on one flat timeout.** The bridge protocol gained ack/progress/chunk/final
  response kinds — an ack proves the plugin got the request, a heartbeat
  proves it's still alive, and results above a size threshold stream back
  as path-addressed chunks reassembled on completion. A sliding inactivity
  timer (short pre-ack, generous once progressing) plus an independent
  stall watchdog replace the old flat 30s cutoff, and a lost ack is retried
  once automatically. On the plugin side, a self-tuning concurrency pool
  caps and throttles tree-serialization fan-out so the heartbeat itself
  never gets starved by its own request's traffic.
- Ground-truth extraction overhaul: component/prop matching now prefers
  data read straight from Figma (instance/main-component name,
  `componentProps`, bound-Variable `codeSyntax`) over the vision model,
  which is now the fallback only for the one question Figma's data model
  can't answer.
- `backend/` (formerly `bridge/server`) promoted to a persistent leader/
  follower backend behind `/api/v1`, with the leader-election layer and the
  plugin's `serializer.ts`/`code.ts` rewritten to drop the vendored fork
  dependency; `bridge/plugin` and `bridge/extension` moved to `extensions/`.
- Rotation sign in codegen's CSS output corrected; render waits for
  `document.fonts.ready` instead of a fixed sleep; a lean structure-only
  serialize mode avoids fetching tokens/styles when only shape is needed.

### Fixed

- Plugin: exporting a node via its nearest background-filled ancestor no
  longer bleeds sibling layers into the crop — they're hidden for the
  duration of the export (with a fallback to exporting the node directly
  when the plugin only has Viewer access and can't hide anything).
- Browser extension: tooltip clipping near viewport corners, history
  thumbnail pin/remove button offsets under a wrapped Tooltip, a missing
  hit-map on Fetch/history load paths, text color leaking through shadow
  DOM into the host page, and generated class names polluting the hover
  selector.
- golangci-lint cleanups (errcheck, revive, unused-parameter) across the Go
  codebase.

### Docs

- Skill (`.claude/skills/figma-map/SKILL.md`) gained a Troubleshooting
  section for the bridge's actual operational failure modes — an
  unfocused/suspended Figma tab, `doctor`'s two separate bridge/plugin
  checks, long-but-not-hung large selections, and a stale process holding
  the bridge port — plus the concrete port (1994) and start commands,
  instead of leaving them implicit.
- README restructured around the agent verify loop rather than component
  mapping, with `init` documented in Quick start, the commands table, and
  MCP integration; ADRs added for the `extensions/` layout, ground-truth
  extraction, and layer-boundaries.
- A reproducible benchmark harness and head-to-head methodology against
  the official Figma MCP.

## [0.2.0] - 2026-06-13

### Added

- **Agent integration** — every operation is now both a CLI command and an MCP
  tool, generated from one registry (`internal/op`) so they cannot drift.
  Run `figma-map mcp` to serve over stdio (official `modelcontextprotocol/go-sdk`).
- `plan` — map every component instance in a frame to a buildable spec (layout,
  imports, props, tokens, honest `unmapped` list).
- `reconcile` — deterministic diff of rendered output vs the design: Figma tokens
  ↔ DOM computed styles, per-element is/should numbers within tolerance
  (`data-figma-node` grounding); optional Tier-2 LLM check (`--semantic`).
- `tokens`, `inspect`, `screenshot`, `export-assets`, `list` operations.
- Design tokens (color/spacing/font/radius/layout) decoded from the Figma tree.
- `--json` output on every command. Deterministic ops no longer require an API key.
- ADR-0001 (figma-map is a dumb tool: deterministic-first, agent owns the loop).

### Changed

- `reconcile` now uses OpenAI **structured outputs** (json_schema, strict) — no
  more parsing JSON out of free text; same for matching and prop inference.
- `reconcile` property coverage expanded: border (width/color), opacity,
  line-height, letter-spacing, text-align, width/height — on top of color, font,
  radius, padding, gap.
- `reconcile` output is now a **report**: fixable vs advisory (content-driven)
  diffs, measurement **coverage**, and `unmeasured` nodes split into actionable
  ("tag this") vs expected (decorative/image). The thing an agent hands a human
  when it can't fully converge.
- Testable seams: `figma.Source` and `llm.VisionModel` are injectable; offline
  tests cover the matcher and the Map/Plan orchestration.
- `figma.Source` methods now take a `context.Context` (cancellation/timeouts
  propagate to bridge HTTP calls).
- Headless Chrome is pooled: one browser is reused across renders (a tab per
  call) instead of launching Chrome on every reconcile.
- **Spatial alignment**: `reconcile` works against an existing, untagged
  implementation — design nodes are matched to DOM elements by geometry/type/text
  when `data-figma-node` is absent (matched-by-position flagged lower-confidence).

### Hardening

- LLM calls retry on 429/5xx/network with exponential backoff.
- The shared headless browser is recreated if it dies; renders retry once.
- reconcile edge cases: letter-spacing `normal` = 0; width/height skipped on
  CSS-transformed elements; missing drop shadow reported; box-shadow/transform
  read from the DOM.
- e2e test exercises the real render → align → diff path against headless Chrome
  (run in CI; skipped where Chrome is absent).

## [0.1.0] - 2026-06-13

### Added

- Initial release.
- `doctor` — verify the figma-bridge backend, headless Chrome, Storybook, and API key.
- `scan` — screenshot Storybook stories into a code-component catalog
  (chromedp), resolving each component's real import from its story source.
- `bind` — match Figma component sections to the catalog with a vision LLM and
  infer each component's prop schema into a reviewable `figma-map.binding.yaml`.
- `map` — identify a Figma node's component and prop values from the binding and
  generate JSX.
- `figma.Source` and `matcher.Matcher` interfaces as extension seams.
- OpenAI-compatible vision client with configurable base URL (OpenAI, gateways,
  local Ollama/llava).
- One-line `install.sh` with OS/arch detection and SHA-256 verification.
- CI (build, test, vet, lint) and GoReleaser-based release pipeline.

[0.3.0]: https://github.com/KirillBaranov/figma-map/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/KirillBaranov/figma-map/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/KirillBaranov/figma-map/releases/tag/v0.1.0

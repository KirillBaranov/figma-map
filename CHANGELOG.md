# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

## [0.1.0] - 2026-06-13

### Added

- Initial release.
- `doctor` — verify the figma-mcp-bridge, headless Chrome, Storybook, and API key.
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

[Unreleased]: https://github.com/KirillBaranov/figma-map/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/KirillBaranov/figma-map/releases/tag/v0.1.0

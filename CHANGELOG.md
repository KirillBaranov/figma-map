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

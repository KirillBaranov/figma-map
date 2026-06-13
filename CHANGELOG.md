# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/KirillBaranov/figma-map/commits/main

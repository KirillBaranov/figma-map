# Fork notice

This directory is a fork of [gethopp/figma-mcp-bridge](https://github.com/gethopp/figma-mcp-bridge)
(MIT License, © GETHOPP LTD — see [LICENSE.md](LICENSE.md)), vendored
in-tree so the Figma plugin/bridge server can evolve in lockstep with
figma-map's Go side (one commit can change the wire protocol on both ends
at once).

Credit to the original GETHOPP LTD authors and contributors for the plugin
and bridge server this builds on.

## What's diverged from upstream so far

- `plugin/src/main/serializer.ts`: serializes `textCase` (UPPER/LOWER/TITLE),
  preserves paint opacity on SOLID fills, surfaces `LAYER_BLUR`/`BACKGROUND_BLUR`
  effects, resolves `explicitVariableModes` to human-readable names
  (`variantModes`), and extracts component variant/property values
  (`componentProps`).
- `plugin/src/main/code.ts`: crops `get_screenshot` exports when the target
  node has no background and an ancestor's background was exported instead.
- A `get_selection` round trip wired into figma-map's `selection` op
  (`internal/figma/bridge.go`, `internal/service/selection.go`).

## What this repo's ongoing plan adds on top

See `/Users/kirillbaranov/.claude/plans/snazzy-waddling-hinton.md` in the
figma-map repo for the full backlog (Phases 0–10): per-node Figma Variable
binding resolution, native `GRID` auto-layout, prototyping reactions,
dropped-style-field fixes, and more.

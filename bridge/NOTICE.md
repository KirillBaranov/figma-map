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

## What this fork adds on top of upstream

- Per-node/per-paint Figma Variable binding resolution (`resolveVariableLabel`,
  `resolveBoundVariables` in `serializer.ts`) plus `codeSyntax`/`scopes`
  passthrough on `get_variable_defs` — so a node's value can be traced back
  to the actual bound Variable instead of the agent guessing.
- Native Figma `GRID` auto-layout support (track sizes/gaps, per-child
  row/column placement).
- Prototyping reactions (trigger + transition type/easing/duration).
- Previously-dropped style fields now serialized: `constraints`,
  `clipsContent`, `blendMode`, `dashPattern`, `cornerSmoothing`,
  per-side stroke weights, auto-layout child escape hatches
  (`layoutPositioning`/`layoutGrow`/`layoutAlign`).
- `devStatus`, `devResources`, `annotations`, `exportSettings` surfaced on
  serialized nodes.

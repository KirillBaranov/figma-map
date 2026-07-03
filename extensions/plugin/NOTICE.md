# Fork notice

This directory is forked from
[gethopp/figma-mcp-bridge](https://github.com/gethopp/figma-mcp-bridge)'s
plugin (MIT License, © GETHOPP LTD — see [LICENSE.md](LICENSE.md)), vendored
in-tree at commit `9ad44d3` (2026-06-28). See
[../../backend/NOTICE.md](../../backend/NOTICE.md) for the bridge-server side
of the same fork, and
[../../backend/UPSTREAM-README.md](../../backend/UPSTREAM-README.md) for the
original project's README.

`../browser/` (the Chrome extension) is not part of the fork — it postdates
the vendor commit entirely and carries no upstream code.

## What's diverged from upstream

- `src/main/serializer.ts`: serializes `textCase` (UPPER/LOWER/TITLE),
  preserves paint opacity on SOLID fills, surfaces
  `LAYER_BLUR`/`BACKGROUND_BLUR` effects, resolves `explicitVariableModes` to
  human-readable names (`variantModes`), and extracts component
  variant/property values (`componentProps`).
- `src/main/code.ts`: crops `get_screenshot` exports when the target node has
  no background and an ancestor's background was exported instead.
- `src/ui/App.tsx`: the in-Figma panel UI, substantially rewritten.
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
  `clipsContent`, `blendMode`, `dashPattern`, `cornerSmoothing`, per-side
  stroke weights, auto-layout child escape hatches
  (`layoutPositioning`/`layoutGrow`/`layoutAlign`).
- `devStatus`, `devResources`, `annotations`, `exportSettings` surfaced on
  serialized nodes.

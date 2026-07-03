# Fork notice

`backend/` includes code forked from
[gethopp/figma-mcp-bridge](https://github.com/gethopp/figma-mcp-bridge)'s
server (MIT License, © GETHOPP LTD — see [LICENSE.md](LICENSE.md)), vendored
in-tree at commit `9ad44d3` (2026-06-28) so the relay/leader-election logic
could evolve in lockstep with figma-map's Go side. The original project's
README is kept at [UPSTREAM-README.md](UPSTREAM-README.md) for reference.

See [../extensions/plugin/NOTICE.md](../extensions/plugin/NOTICE.md) for the
Figma-plugin side of the same fork. `extensions/browser/` is not part of the
fork — it postdates the vendor commit entirely and carries no upstream code.

## Unchanged since the fork

`src/election.ts`, `src/follower.ts`, `src/index.ts`, `src/node.ts` — leader
election, health checks, and role switching. Byte-identical to upstream.
(Tracked separately for a rewrite that would remove this dependency
entirely — see the project's task backlog.)

## Extended from the fork

- `src/bridge.ts`, `src/tools.ts` — lightly modified.
- `src/schema.ts`, `src/types.ts` — extended with figma-map-specific shapes.
- `src/leader.ts` — substantially extended: the original transport-only relay
  now also serves the `/api/v1` REST surface (issues, compare-session,
  compare-history) added per ADR-0003.

## Original to this project (not from the fork)

`src/compareSession.ts`, `src/issues.ts`, `src/reloadSignal.ts`,
`src/store.ts`.

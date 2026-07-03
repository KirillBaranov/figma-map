# ADR-0004: Remove the vendored fork dependency entirely

- Status: accepted
- Date: 2026-07-03

## Context

`backend` (née `bridge/server`, promoted in [ADR-0003](ADR-0003-backend-consolidation.md))
and `bridge/plugin` originated as an in-tree vendor of
[gethopp/figma-mcp-bridge](https://github.com/gethopp/figma-mcp-bridge) (see
the now-removed `bridge/NOTICE.md`). Most of `backend`'s surface —
`bridge.ts`, `leader.ts`, `tools.ts`, `schema.ts`, `types.ts` — had already
been substantially rewritten or extended locally (persistence, `/api/v1`,
the Figma REST source, etc.) and carried no fork attribution burden. Four
files, though, stayed byte-identical to the vendored commit because nothing
had touched them since: `election.ts`, `follower.ts`, `node.ts`, `index.ts`
— the leader-election / role-switching layer that lets multiple `figma-map`
CLI invocations share one Figma WebSocket connection without fighting over
it. Separately, `bridge/plugin/src/main/serializer.ts` and `code.ts` were
*modified* extensions of the vendored code (new fields, new resolvers, new
RPC actions layered onto the original functions) — still built on
substantial portions of upstream, which is a meaningfully different
situation from a from-scratch rewrite: MIT's notice requirement survives
modification, and only stops applying once none of the original expression
remains.

With the project prepping for OSS launch, carrying fork attribution for
code that isn't being kept in sync with upstream is unwanted baggage. The
decision below covers finishing the job for all four files plus both
plugin files, so the whole repo settles on a single top-level `LICENSE`
with no per-directory fork notice.

While reimplementing the backend layer, a pre-existing bug surfaced:
`leader.ts` moved its RPC endpoint to `/api/v1/rpc` as part of the
ADR-0003 versioning work, but `follower.ts` — frozen because it was
vendored — was never updated and still POSTed to the unversioned `/rpc`.
Any follower-role process would 404 on every tool call it tried to proxy
to the leader. This is fixed as part of the rewrite below.

## Decision

Rewrite five files from scratch with equivalent observable behavior but
original code and structure — same wire contracts, same RPC action names,
same JSON field shapes every consumer (Go's `internal/figma`, the browser
extension, the bridge server) already depends on:

- `backend/src/election.ts`, `follower.ts`, `node.ts`, `index.ts` — same
  HTTP surface (leader binds `:1994`, exposes `/ping` for health checks and,
  via `leader.ts` which was already unforked, `/api/v1/rpc` for follower
  proxying) and the same failover characteristics (claim-the-port-or-follow
  on start, jittered 3-5s `/ping` poll, takeover on incumbent silence).
  `follower.ts`'s RPC calls now target `/api/v1/rpc`, matching `leader.ts` —
  fixing the 404 bug described above.
- `bridge/plugin/src/main/serializer.ts`, `code.ts` — same `SerializedNode`
  wire shape, same set of RPC request types and their exact response
  payloads (verified request-type-by-request-type against the pre-rewrite
  build output), same editor-mode/validation error messages. Internally
  restructured: `code.ts`'s single ~2000-line switch became one handler
  function per request type plus a small dispatch table, with the
  try/catch response-envelope logic factored into one wrapper instead of
  living inside the switch.

Verified via:
- `backend`: manual multi-process testing (two processes on one port — one
  binds as leader, the other follows; killing the leader triggers takeover
  within the expected poll window) plus a clean `tsc` build.
- `bridge/plugin`: a clean `tsc --noEmit` against `@figma/plugin-typings`
  (matching the 3 pre-existing type-narrowing quirks already present
  before this rewrite — not introduced by it) and a real `vite build`,
  diffed request-type-by-request-type against the pre-rewrite bundle to
  confirm no RPC action was dropped. This code runs inside the Figma
  plugin sandbox, so it could not be exercised at runtime in this
  environment — a real in-Figma smoke test (open the plugin, run each
  category of tool at least once) is still worth doing before shipping.

No automated test suite exists for `backend` or `bridge/plugin` yet — this
ADR doesn't introduce one, since testing infrastructure for either package
is out of scope here.

## Consequences

- Neither `backend` nor `bridge/plugin` contains any vendored/forked code
  anymore. `bridge/LICENSE.md` and `bridge/NOTICE.md` are deleted outright
  (not just re-scoped) — the repo's single top-level `LICENSE` is now the
  only license file.
- `bridge/extension` was always 100% original (per ADR-0002) and is
  unaffected.
- `backend/package.json` is renamed off `@gethopp/figma-mcp-bridge` to
  `@figma-map/backend`, with repo/homepage/bugs URLs pointed at this repo
  instead of upstream. The standalone product name `figma-mcp-bridge` —
  which had spread into the CLI bin name, the plugin's manifest id and
  package name, warning-message prefixes, and assorted doc/comment
  mentions — is renamed to `figma-bridge` everywhere, matching the name
  already used by `index.ts`'s `McpServer` registration and the README's
  MCP config key. `gethopp/figma-mcp-bridge` mentions inside this ADR and
  ADR-0002 are left as-is: they name the actual upstream GitHub repo as
  historical context, not this project's own naming.
- `docs/adr/ADR-0002-layer-boundaries.md`'s per-layer "Provenance" notes are
  updated in place to point here rather than describing the now-superseded
  fork state.
- Future changes to any of these five files are ordinary local development,
  not "diverging from upstream" — no more tracking obligation against
  `gethopp/figma-mcp-bridge`'s commit history anywhere in this repo.

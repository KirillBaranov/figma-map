# ADR-0003: Backend consolidation — one persistent data plane, thin frontends

- Status: accepted
- Date: 2026-07-03

## Context

[ADR-0002](ADR-0002-layer-boundaries.md) fixed the six-layer boundaries as they
stood at the time, including calling out `bridge/server` as "transport only."
That has quietly stopped being true: `bridge/server` now also owns three
stateful, in-memory stores (`/issues`, `/compare-session`,
`/compare-session/history`) that are wiped on every restart
(`CLAUDE.md`'s own "Restarting the bridge server" section documents this as
expected). It grew a data-plane role without ever being named or scoped as one.

At the same time, two independent clients speak the plugin's RPC wire
protocol: `internal/figma` in Go, and a hand-written `fetch` client in
`bridge/extension/src/background.ts`. They share no types and no contract —
a change to one has no way of being enforced on the other.

The name "bridge" also undersells what this is becoming: not a wire between
two other things, but the one place that should hold durable state and
expose one API to every consumer (CLI, MCP, extension, and — per the source
question below — potentially a Figma-REST-backed headless path).

This ADR fixes the target shape and the migration phases. It does not yet
decide where heavy computation (`matcher`, `reconcile`, `codegen`) executes —
that is called out explicitly as open, tracked for a later ADR/phase.

## Decision

### 1. Promote `bridge/server` to a named backend

`bridge/server` → `backend/` (top-level, sibling to `internal/`, `cmd/`).
It is the single data plane: source adapters (Figma via the plugin
WebSocket), persistence, and — later, pending the open compute question —
derived-artifact caches. `bridge/plugin` and `bridge/extension` keep their
current names/locations for now; renaming them is a mechanical follow-up,
not gated on this ADR.

### 2. One versioned API contract: `/api/v1`

REST for anything with identity and a lifecycle (create/read/update/delete);
RPC-shaped POST endpoints under the same prefix for actions that compute a
result rather than manage a stored resource. One contract, every consumer
(Go's `internal/figma`, the extension) calls it — the extension's hand-rolled
`/rpc` fetches in `background.ts` are retired in favor of this.

```
/api/v1/issues                    GET, POST
/api/v1/issues/:id/ack            POST                (action)
/api/v1/compare-sessions          GET, POST
/api/v1/compare-sessions/:id      GET, DELETE
/api/v1/compare-history           GET, POST
/api/v1/compare-history/:id/pin   POST                (action)
/api/v1/rpc/screenshot            POST { nodeId, fileKey }
/api/v1/rpc/subtree               POST { nodeId, fileKey }
```

### 3. Persistence: flat JSON files, no new runtime dependency

One JSON file per store (`issues.json`, `compare-sessions.json`,
`compare-history.json`), atomic write (write to a temp file, then rename).
Not SQLite: `backend`'s `engines` requirement is plain Node ≥20 — a native
module (`better-sqlite3`) reintroduces exactly the per-OS/arch prebuilt-binary
fragility the Go side avoids by shipping a single checksummed binary, and the
zero-dependency built-in (`node:sqlite`) is only stable from Node 22.5+/24,
not worth bumping the minimum version for data this small and query-simple
(list/filter/find-by-id, no joins).

### 4. Retention: 7-day default TTL, pinned entries exempt

Nothing in `issues`, `compare-sessions`, or `compare-history` should live
long by default — anything older than 7 days is stale and safe to drop,
including un-acked issues (if a week goes by with no reaction, the page or
design has likely already moved on). The one exception is imported Figma
templates/screenshots the user explicitly wants to keep: `compare-history`
already has a `pinned` field for exactly this — pinned entries are exempt
from TTL and are removed only by explicit user action (the existing Remove
button). Newly imported templates/screenshots default to `pinned: true`
rather than requiring the user to pin them manually.

Cleanup is lazy, on store access (whichever request next reads/writes a
store prunes expired, unpinned entries first) — no cron/alarm process needed
to keep the backend "always on" for cleanup to happen on time.

### 5. Figma REST API as an additional, optional `Source` — not a replacement

`figma.Source` is already the seam for this (per ADR-0002). A second
implementation backed by the Figma REST API (Dev Mode/Enterprise-plan-gated)
is worth adding, but strictly additive and strictly read-scoped: it covers
`find`/`inspect`/`tokens`/`screenshot`/`export-assets`/`map`/`plan`/`bind`
(anything that only needs the Figma node tree). It explicitly does **not**
cover `capture issues` / `verify pixeldiff-images` / the compare loop — those
require a live DOM and a live Figma document in sync, which a REST snapshot
cannot provide. This must be documented as a limitation, not silently
unsupported. Audience: headless/server-side agents that can't run an open
Figma desktop session and already hold a paid API token — not a replacement
for the free bridge+extension path, which stays the default.

### 6. Compute location — open, deferred

Whether `matcher`/`reconcile`/`codegen` (today embedded in the CLI-invoked
Go binary via `internal/service`) move into `backend` as a persistent
process, or stay CLI-embedded with `backend` only serving/caching their
results, is **not decided here**. Phases 1–3 below do not depend on this
answer; it is picked up once persistence and the API contract exist to
build on.

## Consequences

- Migration is multi-phase (see implementation plan); nothing above requires
  a big-bang rewrite.
- The extension's client code shrinks: one typed API client instead of a
  hand-rolled `/rpc` fetch layer duplicating Go's wire-protocol knowledge.
- `CLAUDE.md`'s "Restarting the bridge server" section needs updating once
  restart no longer drops state (it currently documents in-memory loss as
  expected/by-design — that stops being true after phase 1).
- README's Architecture section and layer diagram need the `bridge/server` →
  `backend` rename reflected once the move lands.
- This does not change ADR-0001's "dumb tool" stance: adding persistence and
  a real API surface to `backend` is about *where data lives*, not about the
  tool starting to guess or own the agent's loop.

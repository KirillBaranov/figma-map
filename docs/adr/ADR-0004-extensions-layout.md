# ADR-0004: `extensions/` layout and precise fork attribution

- Status: accepted
- Date: 2026-07-03

## Context

[ADR-0003](ADR-0003-backend-consolidation.md) §1 promoted `bridge/server` to
top-level `backend/` and explicitly left `bridge/plugin` and
`bridge/extension` where they were, calling their rename/relocation "a
mechanical follow-up, not gated on this ADR." With `backend/` gone,
`bridge/` no longer had a server in it — it was left holding two client-side
capture surfaces (the Figma plugin and the browser extension) plus the
fork's NOTICE/LICENSE/README, none of which shared a reason to be named
"bridge" once the actual bridge/relay component had moved out.

Separately, auditing exactly what's still forked (vs. original) turned up
that the fork's attribution files were scoped to the whole `bridge/`
directory, which overstated the fork's actual footprint: the browser
extension postdates the vendor commit (`9ad44d3`, 2026-06-28) entirely and
contains zero upstream code, per ADR-0002 §6 ("100% original, not part of
the gethopp fork").

## Decision

### 1. `bridge/plugin` and `bridge/extension` → `extensions/plugin` and `extensions/browser`

Both are client-side capture surfaces, symmetric in role (one has live
access to the Figma document, the other to a live rendered page), grouped
under one parent so the two aren't scattered relative to `backend/`.
`backend/` itself does not move — ADR-0003 §1 stands as written.

### 2. Fork attribution scoped to where the fork actually is

- `backend/NOTICE.md` + `backend/LICENSE.md` — covers the backend side of
  the fork (`election.ts`/`follower.ts`/`index.ts`/`node.ts` unchanged;
  `bridge.ts`/`tools.ts`/`schema.ts`/`types.ts`/`leader.ts` extended).
  `backend/UPSTREAM-README.md` keeps the original project's README for
  reference.
- `extensions/plugin/NOTICE.md` + `extensions/plugin/LICENSE.md` — covers
  the plugin side (`serializer.ts`/`code.ts` extended, `App.tsx`
  substantially rewritten).
- `extensions/browser/` carries no NOTICE/LICENSE — it is not a fork.

### 3. Backend leader-election de-forking is tracked, not done here

`election.ts`, `follower.ts`, `index.ts`, `node.ts` remain byte-identical to
the vendored fork. Rewriting them to drop the fork dependency entirely is
tracked as separate follow-up work, not part of this layout change.

## Consequences

- CI's `bridge` job, `CLAUDE.md`'s extension-reload instructions, and
  README's architecture section/diagram all reference the new
  `extensions/plugin` and `extensions/browser` paths.
- ADR-0002 and ADR-0003's own text is left as an accurate historical
  snapshot of the layout at the time each was written; both get a short
  amendment line pointing here rather than being rewritten in place.

**Amendment (2026-07-03):** §3's tracked follow-up (rewriting
`election.ts`/`follower.ts`/`index.ts`/`node.ts` to drop the fork
dependency) was done independently and in parallel, in a separate PR that
also rewrote `serializer.ts`/`code.ts` — merged into `main` around the same
time as this ADR. See [ADR-0005](ADR-0005-backend-fork-removal.md), which
also removes the NOTICE/LICENSE files §2 added here (and
`backend/UPSTREAM-README.md`/`backend/logo.png`), since none of them
describe a fork that still exists after that rewrite.

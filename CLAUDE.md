# CLAUDE.md

Instructions for Claude Code working in this repo.

## Reloading `bridge/extension` without asking the user

The unpacked Chrome extension can be reloaded from the agent side after a
rebuild — no manual `chrome://extensions` click needed, except once to
bootstrap it.

**How it works:** `bridge/server/src/reloadSignal.ts` is a one-shot flag on
the Leader. `POST /extension/reload` sets it; `GET /extension/reload`
consumes it (returns `{ reload: true }` once, then `false` until requested
again). The extension's `background.ts` polls that GET both piggybacked on
its existing 30s status poll (while any tab has the content script mounted)
and on a `chrome.alarms` tick every 1 minute (works with no tabs open). When
it sees `reload: true`, it calls `chrome.runtime.reload()` on itself —
that's a built-in method requiring no special permission (unlike
`chrome.management`, which manages *other* extensions and needs the
`"management"` permission).

**Usage after a rebuild:**
```bash
cd bridge/server && npm run build
# restart the running bridge process so the new server code is live
cd bridge/extension && npm run build
curl -s -X POST http://localhost:1994/extension/reload
```
Wait ~30-60s, then refresh whatever test tab you're using (already-open tabs
don't get their content script re-injected just from the extension
reloading — same limitation a manual `chrome://extensions` reload has).

**Bootstrapping caveat:** this only works once the *currently running*
`background.js` already contains the polling code. The very first time (or
if this mechanism itself is ever broken by a bad build), one manual reload
in `chrome://extensions` is unavoidable — ask the user to do it once, then
the loop is self-sufficient.

**Don't verify by polling `GET /extension/reload` yourself in a loop** — it's
destructive (consumes the flag on read), so an agent-side poll can steal the
signal before the extension's own poll sees it. To verify a reload actually
happened: POST the request, wait a fixed amount of time without touching the
endpoint (a background `until` loop keyed off elapsed wall-clock time, not
off the endpoint), then check once — if it already reads `false` and you
never consumed it yourself, the extension picked it up.

## Restarting the bridge server

Only one process ever binds `:1994` (whichever `figma-map`/MCP process won
leader election — see `bridge/server/src/node.ts`). To pick up a
`bridge/server` code change:
```bash
lsof -nP -iTCP:1994 -sTCP:LISTEN   # find the current PID
kill <pid>
node bridge/server/dist/index.js & # respawns as LEADER
```
This drops the in-memory issue inbox, compare-session, and compare-history —
expected, documented in each store's own comment (stateless by design, same
as the rest of the bridge).

# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Docker-based e2e test for the install path** (`test/e2e/`, new
  `e2e-install` CI job, `make e2e-install` locally). Builds a "fake
  release" (CLI + backend + plugin) at two versions and runs
  `install.sh` through the full `bridge up` → `doctor` → `update` →
  `uninstall` cycle inside `ubuntu:24.04`, `debian:12`, and `alpine:3.20`
  containers — catching distro-specific shell/libc breakage (`dash` vs
  `ash`, curl vs wget, glibc vs musl) automatically instead of by hand.
  Confirmed Alpine's musl libc can't run the glibc-targeted `bun compile`
  backend binary (`fork/exec: no such file or directory`); the test
  soft-fails that specific check there with a clear diagnostic rather than
  treating it as a hard failure, while still fully asserting install,
  on-disk paths, `update`, and `uninstall` — a real, previously-undetected
  gap the test surfaced on its first run. New `$FIGMA_MAP_BASE_URL`
  override (`install.sh`, `install.ps1`, `internal/release.BaseURL`) points
  every fetch at local fixtures instead of GitHub for this test; a no-op
  in every real install.

- **The backend and Figma plugin are now fetched, not built, by default —
  no git checkout, no Node install required at all.** Previously `bridge
  up` required a full source checkout plus an undocumented `npm install`
  step; now it fetches a standalone, `bun`-compiled backend binary matching
  the running CLI's version (cached under
  `~/.figma-map/versions/<tag>/backend/`) and execs it directly.
  `--repo`/`bridgeRepo` still works exactly as before for contributors
  building from source. New `internal/release` package centralizes
  download/checksum/extract logic shared by the CLI, backend, and plugin
  fetchers.
- **The Figma plugin is unpacked once to a fixed path
  (`~/.figma-map/plugin/`) and refreshed in place on update**, instead of
  needing a fresh download + re-import every time — after `figma-map
  update`, re-running the plugin in Figma picks up the new code without a
  full re-import (`internal/service/plugin.go`'s `EnsurePlugin`).
- **`figma-map update` now owns the whole stack, not just its own
  binary.** After replacing the CLI, it best-effort refreshes the cached
  backend bundle (restarting a bridge that was already running), refreshes
  the Figma plugin bundle in place, and migrates `figma-map.yaml`'s schema
  if needed — printing exactly what changed. New
  `internal/config/migrate.go` adds a `schemaVersion` field and an ordered
  migration mechanism (empty today; infrastructure for the next schema
  change).
- **New `figma-map uninstall` command** — removes the CLI binary, all
  cached backend bundles, the unpacked plugin, and the rest of
  `~/.figma-map`, instead of leaving that to be done by hand.
- **`install.sh`/`install.ps1` now fetch all three components (CLI,
  backend, plugin) in one run**, verify each against its own checksum, and
  print a full summary of what was installed and where, how to update
  (`figma-map update`) and uninstall (`figma-map uninstall`), and a
  ready-to-paste prompt for the human's coding agent.

### Changed

- **The setup skill and README now put the human, not the agent, in charge
  of running the installer.** Autonomously piping a remote script into a
  shell is a pattern many safety-tuned coding agents categorically refuse —
  rather than working around that refusal, `.claude/skills/figma-map-setup/SKILL.md`
  now has the human run `install.sh`/`install.ps1` themselves, and the
  agent's first action is just confirming `figma-map --version` works
  (with explicit guidance for the common "PATH not refreshed in this
  terminal" case). See `docs/onboarding-flow.md` for the full flow,
  including the failure-mode diagrams this was designed against.
- **`figma-map doctor`'s five checks are now explicitly documented as
  blocking vs. optional** (bridge reachable + plugin connected are
  blocking; Chrome, Storybook, and API key are optional) in both SKILL.md
  files, so a partial `doctor` failure doesn't read as "setup isn't done."

### Fixed

- **Every install path pointed at a Figma plugin that couldn't load.**
  `extensions/plugin/dist/` is a build artifact and was never committed
  (it's gitignored), so `manifest.json` in a bare checkout has always
  referenced files that don't exist — even a fresh `git clone` plus the CLI
  release didn't get you a working plugin, and no doc mentioned the missing
  `npm run build` step. Fixed by building the plugin in CI and attaching it
  to every release as `figma-map-plugin.zip` (`manifest.json` + prebuilt
  `dist/`, no Node required to use it) — `README.md`,
  `.claude/skills/figma-map-setup/SKILL.md`, and
  `.claude/skills/figma-map/SKILL.md` now point at that zip instead of the
  checkout's manifest.

## [0.10.0] - 2026-07-16

### Added

- **`install.ps1`** — a PowerShell installer for Windows, mirroring
  `install.sh`: resolves the latest (or a pinned) release, downloads the
  `windows/amd64` archive, verifies its SHA-256 checksum against
  `checksums.txt`, installs to `%LOCALAPPDATA%\figma-map\bin`, and adds that
  directory to the user `PATH` if it's missing. Invoked with
  `irm .../install.ps1 | iex`, same as the existing `curl | sh` one-liner.

### Fixed

- **`figma-map update` now works on Windows.** It previously hard-errored on
  `runtime.GOOS == "windows"` even though CI already publishes a
  `windows/amd64` release archive. Fixed by: downloading and extracting the
  `.zip` release asset goreleaser actually produces for Windows (was always
  requesting `.tar.gz`); and replacing the running `figma-map.exe` by
  renaming it aside first, since Windows allows renaming a running
  executable but not overwriting it directly — the reverse of the
  same-inode rename that works on Unix.
- **`figma-map doctor` no longer misreports Chrome as missing on Windows.**
  `findChrome` only ever checked Unix binary names plus macOS app-bundle
  paths; a stock Windows Chrome install (`%ProgramFiles%`,
  `%ProgramFiles(x86)%`, or `%LocalAppData%`) was never looked at, so
  `doctor` would fail the Chrome check even with Chrome installed.

## [0.9.1] - 2026-07-15

### Added

- **`figma-map-setup` agent skill** (`.claude/skills/figma-map-setup/SKILL.md`)
  — a self-contained bootstrap guide an agent can read and follow to install
  the CLI, start the bridge, walk the human through the one-time Figma
  plugin load, run `init`, and register itself as an MCP server (Claude
  Code, Cursor, Codex CLI) without the human touching a terminal.

### Changed

- **README restructured around a single new-user path**: install → what it
  is / how it works → optional add-ons → reference (manual install,
  troubleshooting, roadmap, docs). Removed the three overlapping copies of
  the setup steps ("Try it in 5 minutes" / "Install" / "Full quick start")
  in favor of one agent-first install section that points at the new setup
  skill, with manual steps and MCP config kept as a linked reference further
  down instead of repeated inline.

## [0.9.0] - 2026-07-15

### Added

- **`codegen` gets a pluggable output target.** Previously JSX/TSX was the
  only thing `codegen` could emit — the tree-walk built JSX text directly,
  and the HTML preview used by `capture`/`pixeldiff` was bolted on via a
  `codeGen.html bool` checked at every builder call site. Replaced with a
  real architecture: the tree-walk now builds a target-neutral `ir.Node`
  tree once (`internal/codegen/ir`), and independent renderer packages
  under `internal/codegen/targets/*` serialize it — `jsx` (default) and
  `htmlrender` (the existing preview, now a real second target instead of a
  bool flag) ship today. Each target self-registers via `init()`, so adding
  Vue/Svelte/Angular later is a new package + one `Register()` call, with no
  change to the tree-walk or to other targets.
- **`--target` flag on `codegen`.** Selects the output renderer explicitly;
  falls back to the project's `figma-map.yaml` `codegen.target`, then to
  `jsx`. An unrecognized target errors with the list of currently
  registered names instead of failing silently.
- **`codegen.target` in `figma-map.yaml`.** A fresh `figma-map init` now
  writes a `codegen:` section defaulted to `target: jsx`, with a comment
  documenting the currently supported renderers — so projects that want a
  different default (once more targets exist) set it once instead of
  passing `--target` on every call.

### Changed

- **`CodegenResult.TSX` → `Code`.** The result field is no longer named
  after a single target; it now carries whichever target's output was
  requested, alongside a new `Target` field naming which renderer produced
  it and a `SchemaVersion` field so external CLI/MCP consumers can detect a
  future breaking change to this result shape instead of silently
  misparsing it.

## [0.8.0] - 2026-07-15

### Added

- **Browser extension: unified logo/icon.** New indigo overlay-square mark
  (`assets/logo.svg` / `assets/logo-mark.svg`) used as the extension's
  toolbar icon (16/32/48/128px, `extensions/browser/icons/`), a small brand
  touch in the Figma plugin UI's footer link, and the README header. The
  extension previously had no `icons` in its manifest at all — just a
  generic placeholder in the toolbar.
- **Browser extension: collapsible "Overlay compare" window.** A minimize
  button next to Close collapses the panel to just its title bar instead of
  unmounting it — keeps in-progress compare state (position, opacity, note)
  instead of losing it on close/reopen.
- **Browser extension: draggable status bar.** The fixed bottom bar (status
  + Select/Compare/Issues/Settings) can now be dragged by its status readout
  to get it out of the way of page content it's covering; double-click to
  reset to the default position. Position persists across page loads.
- **Browser extension: on-site enable is popup-only.** Removed the on-page
  "+" (`EnableFab`) that appeared on every disabled site — the toolbar
  popup's existing "Enable on \<host\>" toggle is now the only way in,
  so a disabled site has zero on-page footprint instead of a floating
  button.
- **Browser extension: Issues list gets an explicit delete action.** The
  existing ack/delete call (already wired to the backend's issue store) was
  labeled "Mark handled" with a checkmark — relabeled to "Delete" with a
  trash icon so the action reads correctly.

### Changed

- **Browser extension: Compare panel decluttered.** Sync-scroll (previously
  a manual toggle) now always on, matching how nearly everyone used it;
  diff mode defaults on (Blend) instead of requiring an extra click every
  time; the buggy screenshot-based "Contrast" diff renderer is now gated
  behind a `CONTRAST_DIFF_ENABLED` flag (off by default) until it's fixed;
  Scale moved under a collapsible "Advanced" section, Opacity stays on the
  main panel. Also dropped redundant icon+text pairing on toggle buttons
  where the text alone already said everything.

### Fixed

- **Browser extension: popup/options pages were silently broken.**
  `vite-plugin-singlefile` inlined the JS bundle directly into
  `popup/index.html` / `options/index.html` as an inline `<script>` —
  which Manifest V3's non-overridable `script-src 'self'` extension-page
  CSP rejects outright, with no visible error beyond a blank/black popup.
  Removed the plugin from both configs (external `<script src>`/`<link>`
  now, which is same-origin and CSP-compliant) and added `base: "./"` so
  the now-external asset paths resolve correctly against
  `dist/popup/index.html` / `dist/options/index.html` (Vite's default
  root-absolute paths would 404 there). This most likely predates this
  release — nothing in the repo history touched these configs since the
  initial extraction commit.
- **Browser extension: popup status flash.** The popup always mounts at
  "pending" and flips to connected/disconnected moments later once the
  real bridge ping resolves; the instant background/color swap read as a
  flicker. Now fades over 150ms.
- **Browser extension: click-triggered flicker on the compare window.**
  `.fm-window`'s mount animation was disabled during drag via a
  `.fm-window-dragging { animation: none }` override — but that class also
  toggles on/off for a plain click (mousedown immediately followed by
  mouseup, no movement), and re-enabling a previously-disabled `animation`
  property restarts it from frame zero. Any click on the window's title bar
  (including its own Minimize/Close buttons) replayed the entrance
  animation. Removed the override.
- **Browser extension: dangling diff-snapshot capture after unmount.**
  `useDiffSnapshot`'s screenshot capture had no guard against the compare
  window unmounting mid-capture (e.g. collapsing it, or disabling the site)
  — the pending `chrome.tabs.captureVisibleTab` promise still resolved and
  called `setState` on a gone component. Added an `isMounted` ref check.

## [0.7.0] - 2026-07-15

### Added

- **`figma inspect` surfaces `TEXT_PATH` curve data.** Figma's "Text on
  Path" nodes have no curve visible through REST, Dev Mode, or static SVG
  export (export flattens the text into per-glyph outlines, no path left) —
  but the Plugin API exposes it as `vectorPaths`/`textPathStartData`, just
  undeclared in the node's typed geometry those other surfaces mirror.
  `inspect` now returns it as a `textPath` field (`vectorPaths[0].data` is
  ready-to-use SVG path data), so the agent can build a real
  `<textPath href="#...">` instead of eyeballing the curve from a
  screenshot.

## [0.6.1] - 2026-07-13

### Fixed

- `cmd/update.go` shipped in 0.6.0 with 18 unchecked-error lint findings
  (best-effort stdout writes, deferred `Close`/`Remove`/`RemoveAll` calls),
  which failed CI on `main` even though the release build itself isn't
  gated by lint. Explicitly discarded — none were actionable failures.

## [0.6.0] - 2026-07-13

### Added

- **`figma-map update`** — downloads the latest release for the current
  platform from GitHub, verifies it against the release's `checksums.txt`,
  and atomically replaces the running binary in place — no need to
  re-run `install.sh`. `--check` reports whether a newer version exists
  without installing; `--force` reinstalls even if already on the target
  version; `--version vX.Y.Z` pins a specific tag. Windows isn't supported
  yet (points at the releases page instead). Only updates the CLI binary
  itself — the bridge backend and Figma plugin still need a manual
  rebuild/reimport when those changed too, and the skill now tells the
  agent to reach for it when the CLI seems out of date.

### Changed

- README rewritten to lead with the benchmark visuals and a three-case
  comparison table (landing hero, landing-hero-2, admin-dashboard: 27-78%
  closer to the design than an eyeballing agent) instead of a single
  number, with the actual by-eye-vs-figma-map methodology spelled out and
  product screenshots for the browser-extension issue flow and the Figma
  bridge panel. Adds a Troubleshooting section led by the most common
  support issue (Figma freezes plugin/WebSocket activity when it loses
  focus or is minimized) and clarifies that only the Figma plugin is a
  hard requirement — Chrome, Storybook, the OpenAI key, and the browser
  extension are each optional, gating one specific feature rather than
  the whole tool. Folder layout, request flow, the full command
  reference, and honest limitations move to `docs/`, linked at the
  bottom.
- Two new benchmark case studies (`bench/cases/landing-hero-2`,
  `bench/cases/admin-dashboard`), indexed in `bench/README.md` alongside
  the original landing-hero case.

### Fixed

- The benchmark comparator rendered every arm at a fixed 900px viewport
  height regardless of the design's actual height, silently cropping
  anything below that and diffing the crop against the design's real
  content in that region. Now renders at the design image's own height.

## [0.5.0] - 2026-07-13

### Added

- **`verify pixeldiff --selector`** — scope the implementation-side
  screenshot to one element (a CSS selector, or a bare Figma node id
  expanded to `[data-figma-node="<id>"]`) instead of the whole viewport, so
  a section that lives mid-page can be diffed against its Figma render
  without setting up isolation for it first. `--width` controls the
  viewport width used for the scoped page render (default 1280 — a scoped
  section's layout is usually driven by its page/container width, not its
  own size, unlike the existing isolated-story path).
- **`capture browser <url> [--selector]`** — the standalone counterpart:
  screenshot a live URL (dev server, Storybook iframe, local HTML file),
  the whole viewport or cropped to one element, for looking at what's
  currently rendered without comparing it against a Figma node. Same
  output convention as `capture screenshot`/`capture render` — writes a
  PNG to `--out`/a default `.figma-map/out/` path, `--inline` for the
  bytes.
- **`figma animation <nodeId>`** — resolves a node's prototyping reactions
  to an actual before/after style delta, not just the trigger/timing
  `figma tokens`' `reactions` field already carries cheaply for every node.
  Follows the reaction's real destination when there is one (ground truth,
  `resolvedVia: "destination"`), or guesses a same-component-set state
  sibling when there isn't (`resolvedVia: "variant-sibling"`, flagged as a
  guess rather than presented as designer-declared), then diffs styles into
  `styleDelta.{from,to}` — enough to write a real CSS `transition`/
  framer-motion `animate` prop instead of just noting that something
  hovers. Deliberately a separate, opt-in call rather than part of
  `reactions`: resolving a destination and diffing full style sets is real
  async work that shouldn't run for every reaction-bearing node a large-file
  tree walk happens to touch. Bridge-only for now (errors on the REST
  source, same as `Selection`).
- `figma tokens`'/`figma inspect`'s `reactions` field now also carries
  `destinationId` (the NODE-navigation action's target) — cheap, since it
  was already being read off the action and discarded.

### Changed

- Skill (`.claude/skills/figma-map/SKILL.md`) rewritten to lead with
  ownership: figma-map's output (`build plan`'s `jsx`, `build codegen`'s
  skeleton, a screenshot) is material to work from, not a deliverable —
  matching the project's own conventions is the agent's job. The
  absolute-vs-flex, icon-export, and trust-the-numbers guidance now explain
  the reasoning behind each call (what Figma's canvas coordinates actually
  imply, why baked vector geometry can't be hand-translated, why a
  structural layout decision isn't something reconcile's property checks
  can catch) instead of thresholds to pattern-match against — prompted by a
  real mis-verst where a decorative, heavily-overlapping icon cluster got
  forced into an evenly-spaced flex row instead of Figma's literal
  coordinates.

## [0.4.0] - 2026-07-10

### Added

- **`bridge up`/`bridge down`/`bridge status`** — start, stop, and check the
  local backend process instead of always requiring a manual `npm --prefix
  backend run build && node backend/dist/index.js`. `up` pings the
  configured bridge URL first and does nothing if something's already
  there (never starts a second copy), otherwise builds it if
  `backend/dist/index.js` doesn't exist yet and spawns it detached so it
  survives this process exiting, polling `/ping` until it's actually
  reachable. `down` stops what `up` started, via a recorded pidfile;
  `status` reports reachability plus that pid and its log path. All three
  are also MCP tools (`bridge_up`/`bridge_down`/`bridge_status`), so an
  agent can start the backend itself instead of asking a human to run a
  shell command. New `bridgeRepo` config field points `up` at the
  figma-map source checkout without needing `--repo` on every call.
  Deliberately not a supervisor: no auto-restart, no health-monitoring
  loop — `doctor`/`bridge status` stay the only source of truth for
  whether it's actually up.

### Changed

- Skill (`.claude/skills/figma-map/SKILL.md`) now points at `bridge up`
  as the first thing to try when the backend isn't running, instead of
  the manual build/start commands (still documented as the fallback when
  `bridgeRepo` isn't configured).

## [0.3.0] - 2026-07-10

### Added

- **`init`** — one-command project onboarding. Scaffolds the bundled Claude
  Code skill, a starter `figma-map.yaml`, and figma-map's MCP server
  registration into a target project's `.mcp.json` (merged in, existing
  servers untouched), plus a re-runnable, delimited section in that
  project's `CLAUDE.md`. Picks the target interactively (fuzzy-filterable)
  or accepts a path for scripted/CI use; always previews what it's about to
  write and asks for confirmation first (`-y` to skip, `--force` to
  overwrite a skill file that's diverged from the bundled version).
- **Figma REST Source** — an optional read-only backend that talks to the
  Figma REST API directly, for ground-truth reads that don't need the
  bridge/plugin round-trip.
- `capture issues` / `capture ack` — an inbox of regions a human flagged via
  the browser extension (screenshot, bounds, CSS selector, optional linked
  Figma node, note), for pairing with `verify pixeldiff-images`.
- `verify pixeldiff-images` — pixel diff between two already-captured
  images directly, no Figma node lookup or browser render needed.
- Browser extension: a bottom bar with per-window state, a per-site
  allowlist (a small "+" by default, the full bar only on enabled sites),
  a hover-selector overlay showing the CSS selector and size, and an
  issue-capture history with pin/remove.
- CSS `var()` emission for Figma Variables that carry a WEB `codeSyntax`,
  instead of always inlining the literal value.
- `.env` is loaded automatically for `OPENAI_API_KEY` (and other secrets) —
  no more requiring it to be exported by hand.
- `find`/`inspect` accept a `--depth` limit, so a large subtree that used to
  time out can be fetched incrementally instead.

### Changed

- **MCP tool schemas now mark only truly-required fields as `required`.**
  Previously every field was required in the generated schema (a JSON
  Schema inference quirk), and worse, the MCP path never applied the same
  `default` tag values the CLI gets from its cobra flags — an MCP caller
  omitting an optional field like `binding`/`catalog` could hit a raw
  `open : no such file or directory` instead of the documented default.
  Both surfaces now agree.
- **Large `get_document`/`get_selection` calls stream instead of blocking
  on one flat timeout.** The bridge protocol gained ack/progress/chunk/final
  response kinds — an ack proves the plugin got the request, a heartbeat
  proves it's still alive, and results above a size threshold stream back
  as path-addressed chunks reassembled on completion. A sliding inactivity
  timer (short pre-ack, generous once progressing) plus an independent
  stall watchdog replace the old flat 30s cutoff, and a lost ack is retried
  once automatically. On the plugin side, a self-tuning concurrency pool
  caps and throttles tree-serialization fan-out so the heartbeat itself
  never gets starved by its own request's traffic.
- Ground-truth extraction overhaul: component/prop matching now prefers
  data read straight from Figma (instance/main-component name,
  `componentProps`, bound-Variable `codeSyntax`) over the vision model,
  which is now the fallback only for the one question Figma's data model
  can't answer.
- `backend/` (formerly `bridge/server`) promoted to a persistent leader/
  follower backend behind `/api/v1`, with the leader-election layer and the
  plugin's `serializer.ts`/`code.ts` rewritten to drop the vendored fork
  dependency; `bridge/plugin` and `bridge/extension` moved to `extensions/`.
- Rotation sign in codegen's CSS output corrected; render waits for
  `document.fonts.ready` instead of a fixed sleep; a lean structure-only
  serialize mode avoids fetching tokens/styles when only shape is needed.

### Fixed

- Plugin: exporting a node via its nearest background-filled ancestor no
  longer bleeds sibling layers into the crop — they're hidden for the
  duration of the export (with a fallback to exporting the node directly
  when the plugin only has Viewer access and can't hide anything).
- Browser extension: tooltip clipping near viewport corners, history
  thumbnail pin/remove button offsets under a wrapped Tooltip, a missing
  hit-map on Fetch/history load paths, text color leaking through shadow
  DOM into the host page, and generated class names polluting the hover
  selector.
- golangci-lint cleanups (errcheck, revive, unused-parameter) across the Go
  codebase.

### Docs

- Skill (`.claude/skills/figma-map/SKILL.md`) gained a Troubleshooting
  section for the bridge's actual operational failure modes — an
  unfocused/suspended Figma tab, `doctor`'s two separate bridge/plugin
  checks, long-but-not-hung large selections, and a stale process holding
  the bridge port — plus the concrete port (1994) and start commands,
  instead of leaving them implicit.
- README restructured around the agent verify loop rather than component
  mapping, with `init` documented in Quick start, the commands table, and
  MCP integration; ADRs added for the `extensions/` layout, ground-truth
  extraction, and layer-boundaries.
- A reproducible benchmark harness and head-to-head methodology against
  the official Figma MCP.

## [0.2.0] - 2026-06-13

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

### Changed

- `reconcile` now uses OpenAI **structured outputs** (json_schema, strict) — no
  more parsing JSON out of free text; same for matching and prop inference.
- `reconcile` property coverage expanded: border (width/color), opacity,
  line-height, letter-spacing, text-align, width/height — on top of color, font,
  radius, padding, gap.
- `reconcile` output is now a **report**: fixable vs advisory (content-driven)
  diffs, measurement **coverage**, and `unmeasured` nodes split into actionable
  ("tag this") vs expected (decorative/image). The thing an agent hands a human
  when it can't fully converge.
- Testable seams: `figma.Source` and `llm.VisionModel` are injectable; offline
  tests cover the matcher and the Map/Plan orchestration.
- `figma.Source` methods now take a `context.Context` (cancellation/timeouts
  propagate to bridge HTTP calls).
- Headless Chrome is pooled: one browser is reused across renders (a tab per
  call) instead of launching Chrome on every reconcile.
- **Spatial alignment**: `reconcile` works against an existing, untagged
  implementation — design nodes are matched to DOM elements by geometry/type/text
  when `data-figma-node` is absent (matched-by-position flagged lower-confidence).

### Hardening

- LLM calls retry on 429/5xx/network with exponential backoff.
- The shared headless browser is recreated if it dies; renders retry once.
- reconcile edge cases: letter-spacing `normal` = 0; width/height skipped on
  CSS-transformed elements; missing drop shadow reported; box-shadow/transform
  read from the DOM.
- e2e test exercises the real render → align → diff path against headless Chrome
  (run in CI; skipped where Chrome is absent).

## [0.1.0] - 2026-06-13

### Added

- Initial release.
- `doctor` — verify the figma-bridge backend, headless Chrome, Storybook, and API key.
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

[0.4.0]: https://github.com/KirillBaranov/figma-map/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/KirillBaranov/figma-map/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/KirillBaranov/figma-map/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/KirillBaranov/figma-map/releases/tag/v0.1.0

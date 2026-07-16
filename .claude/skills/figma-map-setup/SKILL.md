---
name: figma-map-setup
description: Use when a user wants to install or set up figma-map in a project for the first time — installing the CLI, starting the bridge, wiring the MCP server into this agent's config, and running init. Not for day-to-day usage once it's already installed (see the figma-map skill for that).
---

# figma-map-setup: install and connect figma-map, end to end

This bootstraps a project so figma-map's tools are available to you (the
agent) over MCP.

## 0. The human runs the installer — not you

**Do not run `install.sh`/`install.ps1` yourself, and do not pipe a curl/irm
command into a shell on the human's behalf.** Autonomously downloading and
executing a remote script is something many agent safety configurations
categorically refuse — including, possibly, yours. Rather than working
around that, this skill sidesteps it entirely: the human always runs the
installer themselves, and your first real action is just checking that it
already happened.

Tell the human, in these exact terms:

- **macOS/Linux:** `curl -fsSL https://raw.githubusercontent.com/KirillBaranov/figma-map/main/install.sh | sh`
- **Windows (PowerShell, not cmd.exe):** `irm https://raw.githubusercontent.com/KirillBaranov/figma-map/main/install.ps1 | iex`

This installs three things in one run: the `figma-map` CLI, a standalone
backend bundle (no Node install required to run it), and the Figma plugin
(no build step required to load it) — see docs/onboarding-flow.md in the
figma-map repo for the full diagram if you want the detail. Wait for the
human to confirm they ran it before continuing.

## 1. Confirm the CLI is there

```bash
figma-map --version
```

If this fails right after the human says they ran the installer, the fix
is almost always **"open a new terminal"** — PATH changes made by the
installer don't apply to a shell session that was already open. Don't
suggest reinstalling; suggest a fresh terminal, then retry this command.
If it still fails after that, something in the install genuinely broke —
ask the human to paste the installer's output.

## 2. Start the bridge

```bash
figma-map bridge up
```

No `--repo` needed — the backend was already fetched by the installer (or
`bridge up` fetches it itself on first use if it wasn't). `--repo <path to
a figma-map source checkout>` is only for contributors building the
backend from source instead of using a release bundle. This command is
idempotent — safe to run even if something's already listening on `:1994`.

## 3. Ask the human to load the Figma plugin (one-time, manual — you can't do this part)

The installer already unpacked the plugin to a fixed local path (`.figma-map/plugin/manifest.json`
under the human's home directory) — nothing to download or build. Tell the
human, in these exact terms:

1. Open the Figma file (desktop app) they want to work with.
2. **Plugins → Development → Import plugin from manifest…**
3. Select `manifest.json` from `.figma-map/plugin/` in their home directory
   (that's where the installer put it — not a path in any project or
   checkout).
4. Run it once (**Plugins → Development → Figma MAP Bridge**).

Wait for their confirmation before continuing — `doctor` in the next step
will fail until this is done. After a future `figma-map update`, this
becomes just "run it again" in Figma, not a fresh re-import — the plugin's
files get refreshed in place at the same path.

## 4. Wire it into the target project

```bash
figma-map init <path to the target project>
cd <path to the target project> && figma-map doctor
```

`init` registers figma-map as an MCP server in `<project>/.mcp.json`
(merging in, not overwriting other servers already there), drops the usage
skill at `.claude/skills/figma-map/SKILL.md`, writes a starter
`figma-map.yaml`, and appends a short section to `CLAUDE.md`.

`doctor` runs five independent checks — read each by name, not as one
pass/fail verdict:

- **Blocking** (setup isn't done until these pass): bridge reachable,
  and a Figma file actually connected to it (the bridge process can be up
  with zero files connected — that's its own failing check, not a generic
  "unreachable"; the most common cause is Figma being minimized or out of
  focus, since Figma freezes plugin execution when its window isn't
  active — bring it to the foreground and retry).
- **Optional** (fine to ignore for now, see below): headless Chrome,
  Storybook, API key.

## 5. Connect yourself (or another agent) to the MCP server

`init` wrote:

```json
{ "mcpServers": { "figma-map": { "command": "/absolute/path/to/figma-map", "args": ["mcp"] } } }
```

into `.mcp.json` at the project root.

- **If you are Claude Code:** you read project-root `.mcp.json` yourself —
  nothing further to do. If you haven't picked it up yet, tell the user to
  reopen the project (or restart you) and approve the server when prompted.
- **If you are Cursor:** `.mcp.json` isn't read automatically — copy the
  same `command`/`args` pair into `.cursor/mcp.json` in the project (or
  `~/.cursor/mcp.json` globally), same `mcpServers` shape.
- **If you are Codex CLI:** copy it into `~/.codex/config.toml` instead,
  translated to TOML:
  ```toml
  [mcp_servers.figma-map]
  command = "/absolute/path/to/figma-map"
  args = ["mcp"]
  ```
- **Any other MCP-capable agent:** the `command`/`args` pair above is
  everything it needs; adapt it to that agent's own config format.

## 6. Confirm it worked

Call `figma_pages` (or run `figma-map figma pages` if you're not yet
connected over MCP). If it returns the open file's name and page list,
setup is done — hand the user back to the `figma-map` skill for the actual
build/verify loop.

## No coding agent available at all?

Everything above works run by hand, in order, with no agent involved:
install (step 0), `figma-map doctor` to self-check (it's agent-agnostic —
plain-English pass/fail per check), then `figma-map init <path>` and load
the plugin per step 3.

## Optional, don't block on these

- **Storybook + a vision LLM key** (`OPENAI_API_KEY` by default) — only
  needed for `setup scan`/`setup bind`/`build map` (matching a Figma
  instance to *your* code component). Reading tokens/structure from Figma
  and the reconcile verify loop work without either.
- **Browser extension** — lets a human flag a live-page mismatch and hand
  it to you as a Figma-linked issue, plus a pixel-perfect overlay diff with
  auto-scaling. Needs a source checkout: `cd extensions/browser && npm
  install && npm run build`, then load `extensions/browser/dist` unpacked
  in `chrome://extensions`.

---
name: figma-map-setup
description: Use when a user wants to install or set up figma-map in a project for the first time — installing the CLI, starting the bridge, wiring the MCP server into this agent's config, and running init. Not for day-to-day usage once it's already installed (see the figma-map skill for that).
---

# figma-map-setup: install and connect figma-map, end to end

This bootstraps a project so figma-map's tools are available to you (the
agent) over MCP. Run these steps yourself; don't just print them for the
human to run by hand — that's the point of this skill.

## 0. Find the figma-map checkout

You need a local clone of https://github.com/KirillBaranov/figma-map — the
bridge backend and Figma plugin live there, not in the target project. Ask
the user for its path if you don't already know it (or offer to `git clone`
it somewhere if they don't have one yet).

## 1. Install the CLI

```bash
curl -fsSL https://raw.githubusercontent.com/KirillBaranov/figma-map/main/install.sh | sh
```

Confirm it landed on `$PATH` with `figma-map --version`. If it didn't,
either add the install dir to `$PATH` or use the resolved absolute path in
every command below.

## 2. Start the bridge

```bash
figma-map bridge up --repo <path to the figma-map checkout from step 0>
```

This builds and starts the local backend on `:1994`. It's idempotent — safe
to run even if something's already listening there.

## 3. Ask the user to load the Figma plugin (one-time, manual — you can't do this part)

Tell the user, in these exact terms:

1. Open the Figma file (desktop app) they want to work with.
2. **Plugins → Development → Import plugin from manifest…**
3. Select `extensions/plugin/manifest.json` from the figma-map checkout.
4. Run it once (**Plugins → Development → Figma MAP Bridge**).

Wait for their confirmation before continuing — `doctor` in the next step
will fail until this is done.

## 4. Wire it into the target project

```bash
figma-map init <path to the target project>
cd <path to the target project> && figma-map doctor
```

`init` registers figma-map as an MCP server in `<project>/.mcp.json`
(merging in, not overwriting other servers already there), drops the usage
skill at `.claude/skills/figma-map/SKILL.md`, writes a starter
`figma-map.yaml`, and appends a short section to `CLAUDE.md`. `doctor`
verifies the bridge, Chrome, Storybook, and API key are all reachable — a
Storybook/API-key failure is fine to ignore for now, both are optional (see
below).

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

## Optional, don't block on these

- **Storybook + a vision LLM key** (`OPENAI_API_KEY` by default) — only
  needed for `setup scan`/`setup bind`/`build map` (matching a Figma
  instance to *your* code component). Reading tokens/structure from Figma
  and the reconcile verify loop work without either.
- **Browser extension** (`extensions/browser` in the checkout) — lets a
  human flag a live-page mismatch and hand it to you as a Figma-linked
  issue, plus a pixel-perfect overlay diff with auto-scaling. `cd
  extensions/browser && npm install && npm run build`, then load
  `extensions/browser/dist` unpacked in `chrome://extensions`.

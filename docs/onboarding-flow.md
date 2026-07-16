# Onboarding flow

The target install flow, once the single-installer redesign lands (see
[CHANGELOG.md](../CHANGELOG.md) and the `figma-map-plugin.zip` release asset
already shipping today). Renders natively on GitHub — no export step.

Lanes: **Human**, **Installer script** (`install.sh` / `install.ps1`),
**Coding agent**, **Figma**.

Covers three failure categories, not just the happy path:

- **A — pre-CLI failures**: nothing is installed yet, so `doctor` can't help;
  these need their own explicit dead-ends/retries.
- **B — post-install failures**: `doctor`'s five checks are the single
  diagnostic hub — each has its own named fix, and the optional ones
  (Chrome/Storybook/API key) must not block moving on.
- **C — version drift**: `figma-map update` only updates the CLI binary;
  backend/plugin can go stale independently (see the second diagram below).

```mermaid
flowchart TD
    subgraph Human["🧑 Human"]
        H0(["Start"])
        H1["Run the one install command\n(curl | sh or irm | iex)"]
        HshellErr["Wrong shell:\nirm in cmd.exe → command not found"]
        H2["Open coding agent,\npaste the printed prompt"]
        HpathErr["figma-map --version:\ncommand not found"]
        HnewTerm["Open a new terminal\n(PATH only refreshes in a\nnew session)"]
        H3["Import plugin from manifest\nin Figma, run it once"]
        HrefocusFigma["Bring Figma to the\nforeground (it freezes the\nplugin/WebSocket when\nminimized or unfocused)"]
        H4(["Done — hand off to\nfigma-map usage skill"])
    end

    subgraph Installer["📦 Installer script"]
        I1["Detect OS/arch"]
        IplatErr["No release for this\nplatform/arch\n(e.g. windows/arm64)"]
        I2["Download + verify checksum:\nCLI binary, backend bundle,\nplugin zip"]
        IcksumErr["Checksum mismatch or\ndownload failed"]
        I3["Install to standard paths:\nmacOS/Linux: /usr/local/bin (CLI),\n~/.figma-map/versions/vX/ (backend),\n~/.figma-map/plugin/ (plugin)\nWindows: %LOCALAPPDATA%\\figma-map\\bin (CLI),\nsame layout under %LOCALAPPDATA%\\figma-map\\"]
        IpermErr["No write permission\non install dir"]
        I4["Print summary: what + where,\nfigma-map update / uninstall,\n'open a new terminal if not\nfound', agent prompt to paste"]
    end

    subgraph Agent["🤖 Coding agent"]
        A1["figma-map --version\n(already installed — agent\nnever runs the installer itself)"]
        A2["figma-map init &lt;project&gt;"]
        A3["figma-map bridge up"]
        A3PortErr["Port :1994 already used\nby something else"]
        A4{"figma-map doctor —\nread each check by name,\nnot pass/fail overall"}
        A4bridge["bridge unreachable\n→ restart backend"]
        A4plugin["bridge up, but no Figma\nfile connected\n→ Figma focus / re-import"]
        A4chrome["chrome missing (optional)\n→ warn, continue"]
        A4story["storybook down (optional)\n→ warn, continue"]
        A4key["API key unset (optional)\n→ warn, continue"]
        A5["figma_pages — confirm\nconnected to the right file"]
        A6(["Ready"])
    end

    subgraph Figma["🎨 Figma"]
        F1["Plugin loaded,\nconnected over WebSocket"]
    end

    H0 --> H1 --> I1
    I1 -->|unsupported platform| IplatErr --> H1
    I1 -->|ok| I2
    I2 -->|checksum/download fails| IcksumErr --> H1
    I2 -->|ok| I3
    I3 -->|no write access| IpermErr -->|retry elevated| I3
    I3 -->|ok| I4
    I4 --> H2

    H1 -.->|Windows, wrong shell| HshellErr -->|retry in PowerShell| H1

    H2 --> A1
    A1 -->|not on PATH yet| HpathErr --> HnewTerm --> A1
    A1 -->|found| A2 --> A3
    A3 -->|:1994 taken by another process| A3PortErr -->|free the port,\nretry| A3
    A3 -->|ok| A4

    A3 -.->|asks human — can't\ndo this itself| H3
    H3 --> F1
    F1 -.-> A4

    A4 --> A4bridge -->|fixed| A3
    A4 --> A4plugin -.->|"Figma minimized/lost focus"| HrefocusFigma --> H3
    A4 --> A4chrome --> A5
    A4 --> A4story --> A5
    A4 --> A4key --> A5
    A4 -->|all required checks pass| A5 --> A6 --> H4
```

## Version drift — `figma-map update` must own the whole stack, not just the CLI

`figma-map update` today only replaces the binary in place; it doesn't know
whether the backend bundle or the plugin it already fetched are still the
matching version. Left unhandled, this silently reintroduces category-B
failures (a newer CLI talking to a stale backend/plugin) well after the
initial install succeeded. The target design pushes every step that
*can* be automated into `update` itself, so the only thing ever left for
the human is the one thing Figma doesn't let anything else do.

```mermaid
flowchart LR
    U0(["figma-map update runs"]) --> U1["CLI binary replaced\nwith target version"]

    U1 --> U2{"Backend bundle version\nunder ~/.figma-map/versions/\nmatches new CLI?"}
    U2 -->|yes| U3(["No further action"])
    U2 -->|no| U4["Auto: fetch matching backend\nbundle, replace in place,\nrestart it if bridge was running"]
    U4 --> U3

    U1 --> U5{"Plugin bundle version\nunder ~/.figma-map/plugin/\nmatches new CLI?"}
    U5 -->|yes| U3
    U5 -->|no| U6["Auto: fetch new\nfigma-map-plugin.zip, unpack\nover the SAME fixed path\n(~/.figma-map/plugin/) —\nmanifest.json path never changes"]
    U6 --> U7["Only manual step left:\nclick Run again in Figma\n(Plugins → Development →\nFigma MAP Bridge) — NOT a\nfull re-import, since the\nmanifest's path is unchanged"]
    U7 --> U3

    U1 --> U8{"figma-map.yaml /\nfigma-map.binding.yaml\nschema version current?"}
    U8 -->|yes| U3
    U8 -->|no| U9["Auto-migrate: add missing\nfields with defaults, rename\nold keys — print exactly\nwhat changed (auditable,\nnot silent)"]
    U9 --> U3
```

The plugin branch depends on Figma actually picking up on-disk changes when
a dev plugin already imported from a given `manifest.json` path is re-run —
worth confirming against real Figma behavior before relying on it, since
it's what turns "re-download, unzip, re-import" into "click run".

## Why the human runs the installer, not the agent

Piping a remote script into a shell autonomously is a pattern many
safety-tuned coding agents categorically refuse (asking, then treating any
justification the agent gives itself, doesn't help — it's a hard rule for
some). Making the human the one who always runs the installer removes the
failure mode entirely instead of working around it: the agent's first
action is `figma-map --version` against a binary that's already there,
which is a normal invocation, not an execution-of-untrusted-code decision.

## Uninstall / update

- `figma-map update` — fetches and swaps in the latest release for the
  current platform (already shipped, see `cmd/update.go`); see the
  version-drift diagram above for what it does *not* handle yet.
- `figma-map uninstall` — planned; should remove the CLI binary, the
  backend bundle under `~/.figma-map/versions/`, and offer to remove the
  unpacked plugin directory, without requiring the human to remember every
  path by hand.

package main

import "embed"

// Assets embeds the files `init` scaffolds into a target project, so a
// release binary always carries the exact skill/config version matching
// that build — no separate asset to keep in sync.
//
//go:embed .claude/skills/figma-map/SKILL.md
//go:embed figma-map.example.yaml
var Assets embed.FS

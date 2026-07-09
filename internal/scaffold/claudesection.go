package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	claudeFile   = "CLAUDE.md"
	sectionStart = "<!-- figma-map:start -->"
	sectionEnd   = "<!-- figma-map:end -->"
)

// claudeSectionBody is the content upserted between the markers. It's kept
// short and points at the skill for the real detail rather than duplicating
// it here.
const claudeSectionBody = `## figma-map

This project has the figma-map MCP server registered (see ` + "`.mcp.json`" + `)
and its Claude Code skill installed at
` + "`.claude/skills/figma-map/SKILL.md`" + `. Run ` + "`figma-map doctor`" + ` to verify
the bridge, Chrome, Storybook, and API key are all reachable before using it.`

// WriteClaudeSection upserts a delimited figma-map section into
// <target>/CLAUDE.md without disturbing any other content in the file:
//   - missing file: created containing just the section.
//   - file has the start/end markers: only the text between them is
//     replaced, so this is safe to rerun on every init / future upgrade.
//   - file exists with no markers: the section is appended at the end,
//     everything above it left byte-for-byte untouched.
func WriteClaudeSection(target string, preview bool) (string, error) {
	path := filepath.Join(target, claudeFile)
	section := sectionStart + "\n" + claudeSectionBody + "\n" + sectionEnd

	existing, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		if preview {
			return "CLAUDE.md will be created", nil
		}
		if err := os.WriteFile(path, []byte(section+"\n"), 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", path, err)
		}
		return "CLAUDE.md created", nil
	case err != nil:
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	content := string(existing)
	startIdx := strings.Index(content, sectionStart)
	endIdx := strings.Index(content, sectionEnd)

	var updated string
	var status string
	switch {
	case startIdx >= 0 && endIdx > startIdx:
		before := content[:startIdx]
		after := content[endIdx+len(sectionEnd):]
		updated = before + section + after
		status = "CLAUDE.md figma-map section updated"
	default:
		sep := "\n"
		if strings.HasSuffix(content, "\n\n") || content == "" {
			sep = ""
		} else if !strings.HasSuffix(content, "\n") {
			sep = "\n\n"
		}
		updated = content + sep + "\n" + section + "\n"
		status = "CLAUDE.md figma-map section appended"
	}

	if updated == content {
		return "CLAUDE.md figma-map section already up to date", nil
	}
	if preview {
		return status + " (preview)", nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return status, nil
}

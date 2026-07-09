package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const mcpConfigFile = ".mcp.json"

// mcpServerEntry is the subset of an MCP server config figma-map needs to
// register itself; unknown fields already present under mcpServers.<name>
// for OTHER servers are preserved via json.RawMessage in the parent map.
type mcpServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// WriteMCPConfig registers figma-map as an MCP server in
// <target>/.mcp.json, merging into (not overwriting) any existing file so
// other servers already configured there are left untouched. binaryPath
// should be the absolute path to the running figma-map binary, resolved by
// the caller via os.Executable(), so registration works regardless of
// whether the install directory is on $PATH.
func WriteMCPConfig(target, binaryPath string, force, preview bool) (string, error) {
	path := filepath.Join(target, mcpConfigFile)

	doc := map[string]json.RawMessage{}
	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		// Start from an empty document.
	case err != nil:
		return "", fmt.Errorf("read %s: %w", path, err)
	default:
		if err := json.Unmarshal(raw, &doc); err != nil {
			return "", fmt.Errorf("parse %s: %w (fix or remove it before running init)", path, err)
		}
	}

	servers := map[string]json.RawMessage{}
	if rawServers, ok := doc["mcpServers"]; ok {
		if err := json.Unmarshal(rawServers, &servers); err != nil {
			return "", fmt.Errorf("parse %s: mcpServers: %w", path, err)
		}
	}

	if _, exists := servers["figma-map"]; exists && !force {
		return "figma-map already registered in .mcp.json", nil
	}

	_, exists := servers["figma-map"]
	action := "figma-map registered in .mcp.json"
	previewAction := ".mcp.json: figma-map will be registered"
	if exists {
		action = "figma-map registration in .mcp.json updated (--force)"
		previewAction = ".mcp.json: figma-map registration will be updated (--force)"
	}
	if preview {
		return previewAction, nil
	}

	entry, err := json.Marshal(mcpServerEntry{Command: binaryPath, Args: []string{"mcp"}})
	if err != nil {
		return "", fmt.Errorf("marshal mcp server entry: %w", err)
	}
	servers["figma-map"] = entry

	rawServers, err := json.Marshal(servers)
	if err != nil {
		return "", fmt.Errorf("marshal mcpServers: %w", err)
	}
	doc["mcpServers"] = rawServers

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return action, nil
}

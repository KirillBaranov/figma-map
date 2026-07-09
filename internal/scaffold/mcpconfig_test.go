package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteMCPConfig(t *testing.T) {
	t.Run("no .mcp.json creates one", func(t *testing.T) {
		target := t.TempDir()
		if _, err := WriteMCPConfig(target, "/usr/local/bin/figma-map", false, false); err != nil {
			t.Fatal(err)
		}
		doc := readMCPConfig(t, target)
		servers := doc["mcpServers"].(map[string]any)
		fm := servers["figma-map"].(map[string]any)
		if fm["command"] != "/usr/local/bin/figma-map" {
			t.Fatalf("unexpected command: %v", fm["command"])
		}
	})

	t.Run("existing unrelated servers are preserved", func(t *testing.T) {
		target := t.TempDir()
		mustWrite(t, filepath.Join(target, ".mcp.json"),
			`{"mcpServers":{"other-tool":{"command":"foo","args":["bar"]}}}`)

		if _, err := WriteMCPConfig(target, "/bin/figma-map", false, false); err != nil {
			t.Fatal(err)
		}
		doc := readMCPConfig(t, target)
		servers := doc["mcpServers"].(map[string]any)
		if _, ok := servers["figma-map"]; !ok {
			t.Fatal("figma-map should have been added")
		}
		other := servers["other-tool"].(map[string]any)
		if other["command"] != "foo" {
			t.Fatalf("unrelated server was mutated: %v", other)
		}
	})

	t.Run("existing figma-map entry is left alone without force", func(t *testing.T) {
		target := t.TempDir()
		mustWrite(t, filepath.Join(target, ".mcp.json"),
			`{"mcpServers":{"figma-map":{"command":"/old/path","args":["mcp"]}}}`)

		if _, err := WriteMCPConfig(target, "/new/path", false, false); err != nil {
			t.Fatal(err)
		}
		doc := readMCPConfig(t, target)
		servers := doc["mcpServers"].(map[string]any)
		fm := servers["figma-map"].(map[string]any)
		if fm["command"] != "/old/path" {
			t.Fatalf("entry should not change without --force, got %v", fm["command"])
		}
	})

	t.Run("force overwrites only the figma-map key", func(t *testing.T) {
		target := t.TempDir()
		mustWrite(t, filepath.Join(target, ".mcp.json"),
			`{"mcpServers":{"figma-map":{"command":"/old/path","args":["mcp"]},"other":{"command":"x"}}}`)

		if _, err := WriteMCPConfig(target, "/new/path", true, false); err != nil {
			t.Fatal(err)
		}
		doc := readMCPConfig(t, target)
		servers := doc["mcpServers"].(map[string]any)
		fm := servers["figma-map"].(map[string]any)
		if fm["command"] != "/new/path" {
			t.Fatalf("expected overwrite with --force, got %v", fm["command"])
		}
		other := servers["other"].(map[string]any)
		if other["command"] != "x" {
			t.Fatalf("unrelated server mutated: %v", other)
		}
	})

	t.Run("malformed existing file errors instead of overwriting", func(t *testing.T) {
		target := t.TempDir()
		path := filepath.Join(target, ".mcp.json")
		mustWrite(t, path, `{not json`)

		if _, err := WriteMCPConfig(target, "/bin/figma-map", false, false); err == nil {
			t.Fatal("expected an error for malformed .mcp.json")
		}
		got, _ := os.ReadFile(path)
		if string(got) != "{not json" {
			t.Fatal("malformed file should not have been touched")
		}
	})

	t.Run("preview never writes", func(t *testing.T) {
		target := t.TempDir()
		if _, err := WriteMCPConfig(target, "/bin/figma-map", false, true); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(target, ".mcp.json")); !os.IsNotExist(err) {
			t.Fatal("preview should not have written .mcp.json")
		}
	})
}

func readMCPConfig(t *testing.T, target string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(target, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

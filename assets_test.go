package main

import "testing"

// TestAssetsEmbedded guards against a typo'd //go:embed pattern silently
// embedding zero bytes — the one failure mode go:embed doesn't catch at
// compile time.
func TestAssetsEmbedded(t *testing.T) {
	for _, path := range []string{
		".claude/skills/figma-map/SKILL.md",
		"figma-map.example.yaml",
	} {
		data, err := Assets.ReadFile(path)
		if err != nil {
			t.Fatalf("read embedded %s: %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("embedded %s is empty", path)
		}
	}
}

package scaffold

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// SkillPath and ConfigPath are the embedded asset paths within the assets
// embed.FS passed to WriteSkill/WriteConfig, and their destinations under a
// target project.
const (
	skillAssetPath  = ".claude/skills/figma-map/SKILL.md"
	skillTargetPath = ".claude/skills/figma-map/SKILL.md"

	configAssetPath  = "figma-map.example.yaml"
	configTargetPath = "figma-map.yaml"
)

// WriteSkill copies the embedded SKILL.md into target's .claude/skills tree.
// If preview is true, no file is written — the returned status describes
// what would happen. force allows overwriting a target that has diverged
// from the bundled version.
func WriteSkill(assets fs.FS, target string, force, preview bool) (string, error) {
	return writeIfChanged(assets, skillAssetPath, filepath.Join(target, skillTargetPath), force, preview)
}

// WriteConfig scaffolds figma-map.yaml from the embedded example config. It
// never overwrites an existing config, regardless of force.
func WriteConfig(assets fs.FS, target string, preview bool) (string, error) {
	dst := filepath.Join(target, configTargetPath)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Sprintf("%s already exists, left untouched", configTargetPath), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", dst, err)
	}

	if preview {
		return fmt.Sprintf("%s will be created", configTargetPath), nil
	}

	data, err := fs.ReadFile(assets, configAssetPath)
	if err != nil {
		return "", fmt.Errorf("read embedded %s: %w", configAssetPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", dst, err)
	}
	return fmt.Sprintf("%s created", configTargetPath), nil
}

// writeIfChanged writes assets/assetPath to dst, unless dst already holds
// byte-identical content (skip, "up to date") or dst differs and force is
// false (skip, "differs"). preview computes the same status without writing.
func writeIfChanged(assets fs.FS, assetPath, dst string, force, preview bool) (string, error) {
	data, err := fs.ReadFile(assets, assetPath)
	if err != nil {
		return "", fmt.Errorf("read embedded %s: %w", assetPath, err)
	}

	existing, err := os.ReadFile(dst)
	switch {
	case os.IsNotExist(err):
		if preview {
			return fmt.Sprintf("%s will be created", dst), nil
		}
	case err != nil:
		return "", fmt.Errorf("stat %s: %w", dst, err)
	case bytes.Equal(existing, data):
		return fmt.Sprintf("%s already up to date", dst), nil
	case !force:
		return fmt.Sprintf("%s differs from the bundled version — skipped (rerun with --force to overwrite)", dst), nil
	default:
		if preview {
			return fmt.Sprintf("%s will be overwritten (--force)", dst), nil
		}
	}

	// Only reachable with preview == false: either dst didn't exist, or it
	// differed and force was set — both fall through their switch case to
	// perform the actual write below.
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", dst, err)
	}
	return fmt.Sprintf("%s written", dst), nil
}

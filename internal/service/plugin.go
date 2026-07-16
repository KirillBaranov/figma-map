package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/release"
)

// pluginArchiveName is the release asset name for the Figma plugin bundle
// (built and attached by .github/workflows/release.yml — see
// .goreleaser.yaml's release.extra_files).
const pluginArchiveName = "figma-map-plugin.zip"

// pluginZipRoot is the top-level folder the release CI step wraps the
// plugin's manifest.json/dist/ in before zipping (see release.yml's
// "package figma plugin" step: `zip -r figma-map-plugin.zip
// figma-map-plugin`). ExtractZip preserves this prefix, so callers must
// account for it.
const pluginZipRoot = "figma-map-plugin"

// pluginDir is the one fixed, stable location the Figma plugin is unpacked
// to and kept at across updates — so a plugin already imported into Figma
// via "Import from manifest" keeps pointing at the same manifest.json path
// after `figma-map update` refreshes its contents in place.
func pluginDir() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "plugin"), nil
}

// pluginVersionFile records which release tag is currently unpacked at
// pluginDir(), so EnsurePlugin can no-op without re-downloading anything
// when it's already current.
func pluginVersionFile() (string, error) {
	dir, err := pluginDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".version"), nil
}

func readPluginVersion() string {
	path, err := pluginVersionFile()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// EnsurePlugin makes sure the Figma plugin unpacked at pluginDir() matches
// version, fetching and swapping in a fresh copy if it doesn't (or if
// force is set). It reports whether anything changed, so callers can print
// the "re-run the plugin in Figma" reminder only when it actually mattered.
//
// The swap is done in place at a fixed path (not a new directory per
// version) specifically so a plugin already imported into Figma via
// "Import from manifest" keeps working after an update — see
// docs/onboarding-flow.md's version-drift diagram. Whether Figma actually
// picks up the new on-disk contents on a bare "Run again" (vs. requiring a
// fresh re-import) is a real assumption that needs confirming against
// actual Figma behavior, not a guarantee this code can make on its own.
func EnsurePlugin(_ context.Context, version string, force bool) (changed bool, err error) {
	if version == "" || version == "dev" {
		return false, fmt.Errorf("no cached plugin bundle and running a dev build (version=%q) — "+
			"build extensions/plugin from source instead (see README's manual install)", version)
	}
	tag := release.NormalizeTag(version)

	if !force && readPluginVersion() == tag {
		return false, nil
	}

	dir, err := pluginDir()
	if err != nil {
		return false, err
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", parent, err)
	}

	// Extract under the same parent as the final pluginDir() so the final
	// swap is a same-filesystem rename (atomic), mirroring cmd/update.go's
	// staged-binary approach.
	newDir := filepath.Join(parent, "plugin.new")
	oldDir := filepath.Join(parent, "plugin.old")
	_ = os.RemoveAll(newDir) // best-effort cleanup from a previous failed attempt
	_ = os.RemoveAll(oldDir)
	defer func() {
		_ = os.RemoveAll(newDir)
		_ = os.RemoveAll(oldDir)
	}()

	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", newDir, err)
	}

	tmpDir, err := os.MkdirTemp("", "figma-map-plugin-*")
	if err != nil {
		return false, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archivePath, err := release.FetchAndVerifySidecar(tmpDir, release.BaseURL(tag), pluginArchiveName)
	if err != nil {
		return false, fmt.Errorf("fetch figma plugin bundle: %w (no plugin release asset at %s?)", err, tag)
	}
	if err := release.ExtractZip(archivePath, newDir); err != nil {
		return false, fmt.Errorf("extract plugin bundle: %w", err)
	}

	extractedRoot := filepath.Join(newDir, pluginZipRoot)
	if _, err := os.Stat(filepath.Join(extractedRoot, "manifest.json")); err != nil {
		return false, fmt.Errorf("plugin bundle missing manifest.json under %s — unexpected archive layout", pluginZipRoot)
	}
	if err := os.WriteFile(filepath.Join(extractedRoot, ".version"), []byte(tag+"\n"), 0o644); err != nil {
		return false, fmt.Errorf("write plugin version file: %w", err)
	}

	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, oldDir); err != nil {
			return false, fmt.Errorf("move existing plugin dir aside: %w", err)
		}
	}
	if err := os.Rename(extractedRoot, dir); err != nil {
		if _, statErr := os.Stat(oldDir); statErr == nil {
			_ = os.Rename(oldDir, dir) // best-effort restore
		}
		return false, fmt.Errorf("install plugin bundle to %s: %w", dir, err)
	}

	return true, nil
}

// Package release fetches and verifies figma-map release artifacts from
// GitHub Releases — shared by cmd/update.go (the CLI binary itself) and
// internal/service (the backend bundle and Figma plugin bundle), so all
// three components agree on one download/checksum/extract implementation
// instead of three copies drifting apart.
package release

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Repo is the GitHub repository releases are fetched from.
const Repo = "kirillbaranov/figma-map"

// BaseURL is the release-download base URL for a given tag (e.g. "v0.10.0").
// $FIGMA_MAP_BASE_URL overrides it wholesale when set, so e2e tests can
// point every fetch (bridge up, figma-map update, CLI self-update) at a
// local fixture server instead of real GitHub — see test/e2e/. Unset in
// every real install; this is a no-op there.
func BaseURL(tag string) string {
	if v := os.Getenv("FIGMA_MAP_BASE_URL"); v != "" {
		return v
	}
	return "https://github.com/" + Repo + "/releases/download/" + tag
}

// NormalizeTag prefixes version with "v" if it doesn't already have one —
// the canonical tag form used consistently as a cache-directory key and a
// release-download path segment (BuildInfo.Version is unprefixed, e.g.
// "0.10.0"; git tags and install.sh's $TAG are prefixed, e.g. "v0.10.0").
// Every cache path keyed by version (backend bundle dirs, the plugin
// version file) must go through this so Go and the shell installers agree
// on the same on-disk path without either side hardcoding the other's
// convention.
func NormalizeTag(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// LatestTag returns the tag name of the latest GitHub release.
func LatestTag() (string, error) {
	url := "https://api.github.com/repos/" + Repo + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "figma-map")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api returned %s", resp.Status)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("parse github api response: %w", err)
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("github api response had no tag_name")
	}
	return rel.TagName, nil
}

// FetchAndVerify downloads archive from baseURL into dir, verifies it
// against that release's checksums.txt, and returns its local path.
func FetchAndVerify(dir, baseURL, archive string) (archivePath string, err error) {
	archivePath = filepath.Join(dir, archive)
	if err := downloadFile(baseURL+"/"+archive, archivePath); err != nil {
		return "", fmt.Errorf("download %s: %w", archive, err)
	}

	checksumsPath := filepath.Join(dir, "checksums.txt")
	if err := downloadFile(baseURL+"/checksums.txt", checksumsPath); err != nil {
		return "", fmt.Errorf("download checksums.txt: %w", err)
	}

	expected, err := checksumFor(checksumsPath, archive)
	if err != nil {
		return "", err
	}
	actual, err := sha256File(archivePath)
	if err != nil {
		return "", fmt.Errorf("hash downloaded archive: %w", err)
	}
	if expected != actual {
		return "", fmt.Errorf("checksum mismatch for %s: expected %s, got %s", archive, expected, actual)
	}
	return archivePath, nil
}

// FetchAndVerifySidecar downloads archive from baseURL into dir, verifying
// it against a "<archive>.sha256" sidecar file (a plain hex digest) instead
// of a shared checksums.txt. Used for release assets attached via
// goreleaser's extra_files (backend bundles, the Figma plugin zip) — those
// aren't covered by goreleaser's own checksums.txt, which only checksums
// the archives goreleaser itself builds.
func FetchAndVerifySidecar(dir, baseURL, archive string) (archivePath string, err error) {
	archivePath = filepath.Join(dir, archive)
	if err := downloadFile(baseURL+"/"+archive, archivePath); err != nil {
		return "", fmt.Errorf("download %s: %w", archive, err)
	}

	sumPath := filepath.Join(dir, archive+".sha256")
	if err := downloadFile(baseURL+"/"+archive+".sha256", sumPath); err != nil {
		return "", fmt.Errorf("download %s.sha256: %w", archive, err)
	}
	sumData, err := os.ReadFile(sumPath)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(sumData))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty checksum file for %s", archive)
	}
	expected := fields[0]

	actual, err := sha256File(archivePath)
	if err != nil {
		return "", fmt.Errorf("hash downloaded archive: %w", err)
	}
	if expected != actual {
		return "", fmt.Errorf("checksum mismatch for %s: expected %s, got %s", archive, expected, actual)
	}
	return archivePath, nil
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %s", resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, resp.Body)
	return err
}

func checksumFor(checksumsPath, archive string) (string, error) {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archive {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %s", archive)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ExtractBinary extracts a single named binary from a .tar.gz or .zip
// archive into destDir, returning its extracted path.
func ExtractBinary(archivePath, destDir, binaryName string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractBinaryFromZip(archivePath, destDir, binaryName)
	}
	return extractBinaryFromTarGz(archivePath, destDir, binaryName)
}

func extractBinaryFromTarGz(archivePath, destDir, binaryName string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("binary %s not found in archive", binaryName)
		}
		if err != nil {
			return "", err
		}
		if filepath.Base(hdr.Name) != binaryName || hdr.Typeflag != tar.TypeReg {
			continue
		}

		outPath := filepath.Join(destDir, binaryName)
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return "", err
		}
		_ = out.Close()
		return outPath, nil
	}
}

func extractBinaryFromZip(archivePath, destDir, binaryName string) (string, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = zr.Close() }()

	for _, zf := range zr.File {
		if filepath.Base(zf.Name) != binaryName || zf.FileInfo().IsDir() {
			continue
		}

		rc, err := zf.Open()
		if err != nil {
			return "", err
		}

		outPath := filepath.Join(destDir, binaryName)
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			_ = rc.Close()
			return "", err
		}
		_, copyErr := io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if copyErr != nil {
			return "", copyErr
		}
		return outPath, nil
	}
	return "", fmt.Errorf("binary %s not found in archive", binaryName)
}

// ExtractZip extracts every file in a .zip archive into destDir, preserving
// its internal directory structure (used for multi-file bundles like the
// Figma plugin, unlike ExtractBinary which pulls out one named file).
func ExtractZip(archivePath, destDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()

	for _, zf := range zr.File {
		outPath := filepath.Join(destDir, zf.Name) //nolint:gosec // release assets are our own signed CI output, not user-controlled
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

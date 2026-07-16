package cmd

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
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

const updateRepo = "kirillbaranov/figma-map"

func newUpdateCmd(info BuildInfo) *cobra.Command {
	var (
		checkOnly   bool
		force       bool
		wantVersion string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update figma-map to the latest release",
		Long: "Downloads the latest figma-map release for this platform, verifies its checksum, " +
			"and replaces the currently running binary in place.",
		SilenceUsage: true,
		RunE: func(c *cobra.Command, _ []string) error {
			return runUpdate(c, info, checkOnly, force, wantVersion)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only check whether a newer version is available")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if already on the target version")
	cmd.Flags().StringVar(&wantVersion, "version", "", "install a specific version (e.g. v0.6.0) instead of latest")
	return cmd
}

func runUpdate(c *cobra.Command, info BuildInfo, checkOnly, force bool, wantVersion string) error {
	out := c.OutOrStdout()

	target := wantVersion
	if target == "" {
		var err error
		target, err = latestReleaseTag()
		if err != nil {
			return fmt.Errorf("resolve latest version: %w", err)
		}
	}
	if !strings.HasPrefix(target, "v") {
		target = "v" + target
	}

	current := info.Version
	if current != "dev" && !strings.HasPrefix(current, "v") {
		current = "v" + current
	}

	if current != "dev" && !force {
		switch semver.Compare(current, target) {
		case 0:
			_, _ = fmt.Fprintf(out, "already up to date (%s)\n", current)
			return nil
		case 1:
			_, _ = fmt.Fprintf(out, "running %s, which is newer than latest release %s — nothing to do\n", current, target)
			return nil
		}
	}

	if checkOnly {
		if current == "dev" {
			_, _ = fmt.Fprintf(out, "running a dev build; latest release is %s\n", target)
		} else {
			_, _ = fmt.Fprintf(out, "update available: %s -> %s\n", current, target)
		}
		return nil
	}

	installPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running binary: %w", err)
	}
	installPath, err = filepath.EvalSymlinks(installPath)
	if err != nil {
		return fmt.Errorf("resolve running binary path: %w", err)
	}
	installDir := filepath.Dir(installPath)

	if err := checkWritable(installDir); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("%s is not writable: %w\nre-run from an elevated shell, or reinstall with install.ps1", installDir, err)
		}
		return fmt.Errorf("%s is not writable: %w\nre-run with sudo, or reinstall with install.sh", installDir, err)
	}

	_, _ = fmt.Fprintf(out, "downloading figma-map %s for %s/%s...\n", target, runtime.GOOS, runtime.GOARCH)

	tmpDir, err := os.MkdirTemp("", "figma-map-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archivePath, binaryName, err := downloadRelease(tmpDir, target)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "extracting...")
	extractedBinary, err := extractBinary(archivePath, tmpDir, binaryName)
	if err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}

	// Stage the new binary in the same directory as the install target so the
	// final rename is same-filesystem and therefore atomic.
	staged, err := os.CreateTemp(installDir, ".figma-map-update-*")
	if err != nil {
		return fmt.Errorf("stage new binary in %s: %w", installDir, err)
	}
	stagedPath := staged.Name()
	defer func() { _ = os.Remove(stagedPath) }() // no-op once the rename below succeeds

	src, err := os.Open(extractedBinary)
	if err != nil {
		_ = staged.Close()
		return fmt.Errorf("open extracted binary: %w", err)
	}
	_, copyErr := io.Copy(staged, src)
	_ = src.Close()
	_ = staged.Close()
	if copyErr != nil {
		return fmt.Errorf("stage new binary: %w", copyErr)
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}

	if runtime.GOOS == "windows" {
		// Windows won't let us rename a new file over the currently-running
		// .exe directly (the image is locked), but it does allow renaming
		// the running .exe itself aside — Windows only blocks deleting or
		// overwriting the file backing a running image, not moving it.
		oldPath := installPath + ".old"
		_ = os.Remove(oldPath) // best-effort cleanup from a previous update
		if err := os.Rename(installPath, oldPath); err != nil {
			return fmt.Errorf("move running binary aside: %w", err)
		}
		if err := os.Rename(stagedPath, installPath); err != nil {
			_ = os.Rename(oldPath, installPath) // best-effort restore
			return fmt.Errorf("install new binary to %s: %w", installPath, err)
		}
		_ = os.Remove(oldPath) // best-effort; harmless if still locked, cleaned up on next update
	} else {
		// Renaming over the running executable is safe on Unix: the process
		// keeps its already-open inode; the path just starts pointing at the
		// new file for the next launch.
		if err := os.Rename(stagedPath, installPath); err != nil {
			return fmt.Errorf("install new binary to %s: %w", installPath, err)
		}
	}

	_, _ = fmt.Fprintf(out, "updated %s -> %s (%s)\n", current, target, installPath)
	if runtime.GOOS == "windows" {
		_, _ = fmt.Fprintln(out, "note: any already-running figma-map backend keeps the old code until restarted "+
			`(netstat -ano | findstr :1994, then taskkill /PID <pid> /F and let it respawn)`)
	} else {
		_, _ = fmt.Fprintln(out, "note: any already-running figma-map backend keeps the old code until restarted "+
			"(lsof -nP -iTCP:1994 -sTCP:LISTEN, then kill it and let it respawn)")
	}
	return nil
}

func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".figma-map-write-test-*")
	if err != nil {
		return err
	}
	_ = f.Close()
	return os.Remove(f.Name())
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func latestReleaseTag() (string, error) {
	url := "https://api.github.com/repos/" + updateRepo + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "figma-map-update")
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

// downloadRelease fetches the release archive for the current platform into
// dir, verifies it against the release's checksums.txt, and returns its path
// plus the binary name to look for inside it.
func downloadRelease(dir, tag string) (archivePath, binaryName string, err error) {
	binaryName = "figma-map"
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		binaryName = "figma-map.exe"
		ext = "zip"
	}
	version := strings.TrimPrefix(tag, "v")
	archive := fmt.Sprintf("%s_%s_%s_%s.%s", "figma-map", version, runtime.GOOS, runtime.GOARCH, ext)
	baseURL := "https://github.com/" + updateRepo + "/releases/download/" + tag

	archivePath = filepath.Join(dir, archive)
	if err := downloadFile(baseURL+"/"+archive, archivePath); err != nil {
		return "", "", fmt.Errorf("download %s (no release asset for %s/%s at %s?): %w",
			archive, runtime.GOOS, runtime.GOARCH, tag, err)
	}

	checksumsPath := filepath.Join(dir, "checksums.txt")
	if err := downloadFile(baseURL+"/checksums.txt", checksumsPath); err != nil {
		return "", "", fmt.Errorf("download checksums.txt: %w", err)
	}

	expected, err := checksumFor(checksumsPath, archive)
	if err != nil {
		return "", "", err
	}
	actual, err := sha256File(archivePath)
	if err != nil {
		return "", "", fmt.Errorf("hash downloaded archive: %w", err)
	}
	if expected != actual {
		return "", "", fmt.Errorf("checksum mismatch for %s: expected %s, got %s", archive, expected, actual)
	}

	return archivePath, binaryName, nil
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

func extractBinary(archivePath, destDir, binaryName string) (string, error) {
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

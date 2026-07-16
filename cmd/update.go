package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/release"
	"github.com/kirillbaranov/figma-map/internal/service"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

func newUpdateCmd(info BuildInfo, get func() *service.Service) *cobra.Command {
	var (
		checkOnly   bool
		force       bool
		wantVersion string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update figma-map to the latest release",
		Long: "Downloads the latest figma-map release for this platform, verifies its checksum, " +
			"replaces the currently running binary in place, and refreshes the backend bundle, " +
			"the Figma plugin, and figma-map.yaml's schema to match.",
		SilenceUsage: true,
		RunE: func(c *cobra.Command, _ []string) error {
			return runUpdate(c, info, get(), checkOnly, force, wantVersion)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only check whether a newer version is available")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if already on the target version")
	cmd.Flags().StringVar(&wantVersion, "version", "", "install a specific version (e.g. v0.6.0) instead of latest")
	return cmd
}

func runUpdate(c *cobra.Command, info BuildInfo, svc *service.Service, checkOnly, force bool, wantVersion string) error {
	out := c.OutOrStdout()

	target := wantVersion
	if target == "" {
		var err error
		target, err = release.LatestTag()
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

	archivePath, binaryName, err := downloadCLIRelease(tmpDir, target)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "extracting...")
	extractedBinary, err := release.ExtractBinary(archivePath, tmpDir, binaryName)
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

	ctx := c.Context()
	refreshBackend(ctx, out, svc, target)
	refreshPlugin(ctx, out, target)
	migrateConfig(out)

	return nil
}

// refreshBackend re-fetches the backend bundle for target if it isn't
// already cached, restarting a bridge that was already running so it picks
// up the new code — best-effort: a failure here doesn't undo the CLI
// update that already succeeded, it just leaves the old backend running
// (same as before this existed) with a warning printed.
func refreshBackend(ctx context.Context, out io.Writer, svc *service.Service, target string) {
	wasRunning := false
	if svc != nil {
		if status, err := svc.BridgeStatus(ctx); err == nil {
			wasRunning = status.Running
		}
	}

	_, changed, err := service.EnsureBackendBundle(ctx, target)
	if err != nil {
		_, _ = fmt.Fprintf(out, "warning: could not refresh backend bundle: %v\n", err)
		return
	}
	if !changed {
		_, _ = fmt.Fprintln(out, "backend bundle already current")
		return
	}
	if !wasRunning || svc == nil {
		_, _ = fmt.Fprintln(out, "backend bundle updated (not currently running)")
		return
	}
	if _, err := svc.BridgeDown(ctx); err != nil {
		_, _ = fmt.Fprintf(out, "warning: backend bundle updated, but could not stop the running backend to restart it: %v\n", err)
		return
	}
	if _, err := svc.BridgeUp(ctx, ""); err != nil {
		_, _ = fmt.Fprintf(out, "warning: backend bundle updated, but restart failed: %v\n", err)
		return
	}
	_, _ = fmt.Fprintln(out, "backend bundle updated and restarted")
}

// refreshPlugin re-fetches the Figma plugin bundle for target in place if
// it isn't already current — best-effort, same as refreshBackend.
func refreshPlugin(ctx context.Context, out io.Writer, target string) {
	changed, err := service.EnsurePlugin(ctx, target, false)
	if err != nil {
		_, _ = fmt.Fprintf(out, "warning: could not refresh Figma plugin bundle: %v\n", err)
		return
	}
	if !changed {
		_, _ = fmt.Fprintln(out, "Figma plugin bundle already current")
		return
	}
	_, _ = fmt.Fprintln(out, "Figma plugin bundle updated — in Figma, re-run it "+
		"(Plugins → Development → Figma MAP Bridge); no need to re-import, its path hasn't changed")
}

// migrateConfig applies any pending figma-map.yaml schema migrations in the
// current directory — best-effort, same as refreshBackend/refreshPlugin.
func migrateConfig(out io.Writer) {
	applied, err := config.Migrate("figma-map.yaml")
	if err != nil {
		_, _ = fmt.Fprintf(out, "warning: could not migrate figma-map.yaml: %v\n", err)
		return
	}
	if len(applied) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "migrated figma-map.yaml:")
	for _, desc := range applied {
		_, _ = fmt.Fprintf(out, "  - %s\n", desc)
	}
}

func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".figma-map-write-test-*")
	if err != nil {
		return err
	}
	_ = f.Close()
	return os.Remove(f.Name())
}

// downloadCLIRelease fetches the figma-map CLI release archive for the
// current platform into dir, verifying it against checksums.txt, and
// returns its path plus the binary name to look for inside it.
func downloadCLIRelease(dir, tag string) (archivePath, binaryName string, err error) {
	binaryName = "figma-map"
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		binaryName = "figma-map.exe"
		ext = "zip"
	}
	version := strings.TrimPrefix(tag, "v")
	archive := fmt.Sprintf("figma-map_%s_%s_%s.%s", version, runtime.GOOS, runtime.GOARCH, ext)

	archivePath, err = release.FetchAndVerify(dir, release.BaseURL(tag), archive)
	if err != nil {
		return "", "", fmt.Errorf("%w (no release asset for %s/%s at %s?)", err, runtime.GOOS, runtime.GOARCH, tag)
	}
	return archivePath, binaryName, nil
}

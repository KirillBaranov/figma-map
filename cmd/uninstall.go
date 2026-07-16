package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kirillbaranov/figma-map/internal/scaffold"
	"github.com/kirillbaranov/figma-map/internal/service"
	"github.com/spf13/cobra"
)

// newUninstallCmd removes everything a figma-map install created: the CLI
// binary itself, cached backend bundles, the unpacked Figma plugin, and the
// rest of ~/.figma-map's state — so uninstalling doesn't mean hunting down
// paths by hand. Hand-wired in cmd/root.go like init/update, since it's an
// interactive-by-default terminal action, not an MCP-tool-shaped operation.
func newUninstallCmd(get func() *service.Service) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the figma-map CLI, cached backend/plugin bundles, and local state",
		Long: "Stops a running bridge, removes ~/.figma-map (cached backend bundles, the " +
			"unpacked Figma plugin, pidfile/log), then removes the figma-map binary itself.",
		SilenceUsage: true,
		RunE: func(c *cobra.Command, _ []string) error {
			return runUninstall(c, get(), yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func runUninstall(c *cobra.Command, svc *service.Service, yes bool) error {
	out := c.OutOrStdout()

	installPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running binary: %w", err)
	}
	installPath, err = filepath.EvalSymlinks(installPath)
	if err != nil {
		return fmt.Errorf("resolve running binary path: %w", err)
	}

	stateDir, err := service.StateDir()
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "figma-map uninstall will remove:")
	_, _ = fmt.Fprintf(out, "  · %s (the CLI binary)\n", installPath)
	if _, statErr := os.Stat(stateDir); statErr == nil {
		_, _ = fmt.Fprintf(out, "  · %s (cached backend bundles, the unpacked Figma plugin, pidfile/log)\n", stateDir)
	}

	if !yes {
		ok, err := scaffold.Confirm("Proceed?")
		if err != nil {
			if err == scaffold.ErrCancelled {
				_, _ = fmt.Fprintln(out, "cancelled — nothing removed")
				return nil
			}
			return err
		}
		if !ok {
			_, _ = fmt.Fprintln(out, "cancelled — nothing removed")
			return nil
		}
	}

	ctx := c.Context()
	if svc != nil {
		if status, err := svc.BridgeStatus(ctx); err == nil && status.Running {
			if _, err := svc.BridgeDown(ctx); err != nil {
				_, _ = fmt.Fprintf(out, "warning: could not stop the running backend: %v\n", err)
			}
		}
	}

	if _, statErr := os.Stat(stateDir); statErr == nil {
		if err := os.RemoveAll(stateDir); err != nil {
			return fmt.Errorf("remove %s: %w", stateDir, err)
		}
		_, _ = fmt.Fprintf(out, "removed %s\n", stateDir)
	}

	if err := removeSelf(installPath); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "removed %s\n", installPath)
	return nil
}

// removeSelf deletes the running binary. On Unix this is a plain unlink —
// removing a file doesn't affect a process that already has it open (same
// inode-unlink reasoning as cmd/update.go's rename-over-running-binary).
// Windows won't allow deleting a running image's backing file at all, only
// renaming it aside (same constraint update.go's install step hits) — so
// there we rename to a sidecar path and tell the human it needs a manual
// delete once the process has actually exited.
func removeSelf(installPath string) error {
	if runtime.GOOS != "windows" {
		return os.Remove(installPath)
	}
	pending := installPath + ".uninstall-pending"
	_ = os.Remove(pending) // best-effort cleanup from a previous uninstall attempt
	if err := os.Rename(installPath, pending); err != nil {
		return fmt.Errorf("move %s aside for removal: %w", installPath, err)
	}
	fmt.Printf("note: Windows can't delete a running .exe — renamed it to %s; "+
		"delete that file once this process exits\n", pending)
	return nil
}

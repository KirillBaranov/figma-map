package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kirillbaranov/figma-map/internal/release"
)

// BridgeUpResult reports what `bridge up` did or found.
type BridgeUpResult struct {
	AlreadyRunning bool   `json:"alreadyRunning"`
	Started        bool   `json:"started"`
	PID            int    `json:"pid,omitempty"`
	Bridge         string `json:"bridge"`
	LogPath        string `json:"logPath,omitempty"`
}

// BridgeDownResult reports what `bridge down` did.
type BridgeDownResult struct {
	Stopped bool   `json:"stopped"`
	PID     int    `json:"pid,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// BridgeStatusResult reports whether the backend `bridge up` manages is
// reachable right now.
type BridgeStatusResult struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Bridge  string `json:"bridge"`
	LogPath string `json:"logPath,omitempty"`
}

const bridgeUpPollTimeout = 8 * time.Second

// BridgeUp starts the backend if nothing is already listening on cfg.Bridge,
// and detaches it so it outlives this process. It never starts a second
// copy: if the bridge already answers /ping, this is a no-op.
//
// If repo (or cfg.BridgeRepo) is set, it builds and runs from that source
// checkout — the contributor/source-override path, unchanged from before.
// Otherwise it fetches (and caches) a prebuilt backend bundle matching the
// running CLI's own version and execs that directly: no checkout, no Node
// on PATH, no npm build step required.
func (s *Service) BridgeUp(ctx context.Context, repo string) (BridgeUpResult, error) {
	if err := pingBridge(ctx, s.cfg.Bridge); err == nil {
		return BridgeUpResult{AlreadyRunning: true, Bridge: s.cfg.Bridge}, nil
	}

	if repo == "" {
		repo = s.cfg.BridgeRepo
	}

	p := progressFrom(ctx)

	var cmdPath string
	var cmdArgs []string
	var workDir string

	if repo != "" {
		if _, err := os.Stat(filepath.Join(repo, "backend", "package.json")); err != nil {
			return BridgeUpResult{}, fmt.Errorf("no backend/package.json under %s — is this a figma-map checkout?", repo)
		}
		distEntry := filepath.Join(repo, "backend", "dist", "index.js")
		if _, err := os.Stat(distEntry); err != nil {
			p.emit("Building backend (backend/dist/index.js not found)…")
			if err := runBuild(repo); err != nil {
				return BridgeUpResult{}, err
			}
		}
		if _, err := exec.LookPath("node"); err != nil {
			return BridgeUpResult{}, fmt.Errorf("node not found on PATH — required to run the backend: %w", err)
		}
		cmdPath = "node"
		cmdArgs = []string{"backend/dist/index.js"}
		workDir = repo
	} else {
		p.emit("Fetching backend bundle…")
		binPath, err := ensureBackendBundle(ctx, s.version)
		if err != nil {
			return BridgeUpResult{}, fmt.Errorf(
				"no bridge repo configured and could not fetch a prebuilt backend bundle: %w — "+
					"pass --repo, or set bridgeRepo in figma-map.yaml, to build from a source checkout instead", err)
		}
		cmdPath = binPath
		workDir = filepath.Dir(binPath)
	}

	logPath, err := bridgeLogPath()
	if err != nil {
		return BridgeUpResult{}, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return BridgeUpResult{}, fmt.Errorf("open %s: %w", logPath, err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command(cmdPath, cmdArgs...) //nolint:gosec // "node" or our own fetched+checksum-verified binary
	cmd.Dir = workDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = detachAttr()

	if err := cmd.Start(); err != nil {
		return BridgeUpResult{}, fmt.Errorf("start backend: %w", err)
	}
	pid := cmd.Process.Pid
	// Release: we're not going to Wait() on a detached child, and don't
	// want it reaped as a zombie tied to this process's lifetime.
	if err := cmd.Process.Release(); err != nil {
		p.emit(fmt.Sprintf("warning: release backend process: %v", err))
	}

	if err := writePID(pid); err != nil {
		p.emit(fmt.Sprintf("warning: could not write pidfile: %v", err))
	}

	p.emit(fmt.Sprintf("Started backend (pid %d), waiting for it to answer %s/ping…", pid, s.cfg.Bridge))
	deadline := time.Now().Add(bridgeUpPollTimeout)
	for time.Now().Before(deadline) {
		if err := pingBridge(ctx, s.cfg.Bridge); err == nil {
			return BridgeUpResult{Started: true, PID: pid, Bridge: s.cfg.Bridge, LogPath: logPath}, nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return BridgeUpResult{}, fmt.Errorf(
		"backend started (pid %d) but never answered %s/ping within %s — check %s",
		pid, s.cfg.Bridge, bridgeUpPollTimeout, logPath)
}

// BridgeDown stops the backend process started by a prior `bridge up`, via
// its recorded pidfile. It's a no-op (not an error) if there's no pidfile
// or the process it names is already gone.
func (s *Service) BridgeDown(_ context.Context) (BridgeDownResult, error) {
	pid, err := readPID()
	if err != nil {
		return BridgeDownResult{Stopped: false, Reason: "no pidfile — not started by `bridge up`, or already stopped"}, nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = removePIDFile()
		return BridgeDownResult{Stopped: false, PID: pid, Reason: fmt.Sprintf("process not found: %v", err)}, nil
	}
	if err := proc.Kill(); err != nil && !strings.Contains(err.Error(), "process already finished") {
		return BridgeDownResult{}, fmt.Errorf("stop pid %d: %w", pid, err)
	}
	_ = removePIDFile()
	return BridgeDownResult{Stopped: true, PID: pid}, nil
}

// BridgeStatus reports whether the bridge answers /ping right now, plus the
// pid `bridge up` recorded (if any) and where its log lives — independent
// of cfg.Figma.Source, since this is specifically about the backend
// process, not which figma.Source the rest of the tool is configured to use.
func (s *Service) BridgeStatus(ctx context.Context) (BridgeStatusResult, error) {
	pid, _ := readPID() // best-effort; 0 if absent/unreadable
	logPath, _ := bridgeLogPath()
	running := pingBridge(ctx, s.cfg.Bridge) == nil
	return BridgeStatusResult{Running: running, PID: pid, Bridge: s.cfg.Bridge, LogPath: logPath}, nil
}

func runBuild(repo string) error {
	cmd := exec.Command("npm", "--prefix", "backend", "run", "build") //nolint:gosec // fixed args
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm --prefix backend run build failed: %w\n%s", err, out)
	}
	return nil
}

// backendBinaryName is the name of the standalone, bun-compiled backend
// binary inside its release archive — self-contained, no Node runtime
// needed to execute it.
func backendBinaryName() string {
	if runtime.GOOS == "windows" {
		return "figma-bridge.exe"
	}
	return "figma-bridge"
}

// backendBundleDir is where a fetched backend bundle for a given release
// tag is cached — home-scoped like stateDir(), keyed by tag (always the
// "v"-prefixed form, via release.NormalizeTag) so it matches exactly the
// directory name install.sh/install.ps1 write to (they only ever have the
// tag on hand, never the unprefixed BuildInfo.Version) and so an update to
// a newer CLI doesn't silently run a stale backend.
func backendBundleDir(tag string) (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "versions", tag, "backend"), nil
}

// EnsureBackendBundle returns the path to a cached backend binary matching
// version (CLI-version form, e.g. "0.10.0" or "v0.10.0" — normalized
// internally), fetching and verifying it from the matching GitHub release
// if it isn't already cached. changed reports whether a fresh fetch
// happened, so callers (like `figma-map update`) know whether a running
// bridge needs restarting.
func EnsureBackendBundle(ctx context.Context, version string) (path string, changed bool, err error) {
	dir, err := backendBundleDir(release.NormalizeTag(version))
	if err != nil {
		return "", false, err
	}
	binPath := filepath.Join(dir, backendBinaryName())
	alreadyCached := fileExists(binPath)

	path, err = ensureBackendBundle(ctx, version)
	if err != nil {
		return "", false, err
	}
	return path, !alreadyCached, nil
}

// ensureBackendBundle is the actual fetch-if-missing logic.
func ensureBackendBundle(_ context.Context, version string) (string, error) {
	if version == "" || version == "dev" {
		return "", fmt.Errorf("no cached backend bundle and running a dev build (version=%q) — "+
			"pass --repo to build from a source checkout instead", version)
	}
	tag := release.NormalizeTag(version)

	dir, err := backendBundleDir(tag)
	if err != nil {
		return "", err
	}
	binPath := filepath.Join(dir, backendBinaryName())
	if fileExists(binPath) {
		return binPath, nil
	}

	tmpDir, err := os.MkdirTemp("", "figma-map-backend-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	archive := fmt.Sprintf("figma-map-backend_%s_%s_%s.%s", strings.TrimPrefix(tag, "v"), runtime.GOOS, runtime.GOARCH, ext)

	archivePath, err := release.FetchAndVerifySidecar(tmpDir, release.BaseURL(tag), archive)
	if err != nil {
		return "", fmt.Errorf("%w (no backend release asset for %s/%s at %s?)", err, runtime.GOOS, runtime.GOARCH, tag)
	}

	extracted, err := release.ExtractBinary(archivePath, tmpDir, backendBinaryName())
	if err != nil {
		return "", fmt.Errorf("extract backend bundle: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	src, err := os.Open(extracted)
	if err != nil {
		return "", err
	}
	out, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		_ = src.Close()
		return "", err
	}
	_, copyErr := io.Copy(out, src)
	_ = src.Close()
	_ = out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if err := os.Chmod(binPath, 0o755); err != nil {
		return "", err
	}
	return binPath, nil
}

func pingBridge(ctx context.Context, bridgeURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bridgeURL+"/ping", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge /ping returned %d", resp.StatusCode)
	}
	return nil
}

// StateDir is where figma-map keeps all machine-wide state: the bridge
// pidfile/log, cached backend bundles (versions/), and the unpacked Figma
// plugin (plugin/). Unlike stateDir(), it does not create the directory —
// callers that only need to read or remove it (like `figma-map uninstall`)
// shouldn't have the side effect of creating it first.
func StateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".figma-map"), nil
}

func stateDir() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	return dir, nil
}

func pidFilePath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bridge.pid"), nil
}

func bridgeLogPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bridge.log"), nil
}

func writePID(pid int) error {
	path, err := pidFilePath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

func readPID() (int, error) {
	path, err := pidFilePath()
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func removePIDFile() error {
	path, err := pidFilePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// BridgeUp starts the backend from repo (falling back to cfg.BridgeRepo)
// if nothing is already listening on cfg.Bridge — building it first if
// backend/dist/index.js doesn't exist yet — and detaches it so it outlives
// this process. It never starts a second copy: if the bridge already
// answers /ping, this is a no-op.
func (s *Service) BridgeUp(ctx context.Context, repo string) (BridgeUpResult, error) {
	if err := pingBridge(ctx, s.cfg.Bridge); err == nil {
		return BridgeUpResult{AlreadyRunning: true, Bridge: s.cfg.Bridge}, nil
	}

	if repo == "" {
		repo = s.cfg.BridgeRepo
	}
	if repo == "" {
		return BridgeUpResult{}, fmt.Errorf(
			"no bridge repo configured — pass --repo, or set bridgeRepo in figma-map.yaml, " +
				"to the figma-map source checkout containing backend/")
	}
	if _, err := os.Stat(filepath.Join(repo, "backend", "package.json")); err != nil {
		return BridgeUpResult{}, fmt.Errorf("no backend/package.json under %s — is this a figma-map checkout?", repo)
	}

	p := progressFrom(ctx)
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

	logPath, err := bridgeLogPath()
	if err != nil {
		return BridgeUpResult{}, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return BridgeUpResult{}, fmt.Errorf("open %s: %w", logPath, err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command("node", "backend/dist/index.js") //nolint:gosec // fixed args, repo-controlled cwd
	cmd.Dir = repo
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

// stateDir is where figma-map keeps machine-wide bridge-process state
// (pidfile, log) — home-scoped, not project-scoped, since only one bridge
// process can bind its port on a given machine at a time.
func stateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	dir := filepath.Join(home, ".figma-map")
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

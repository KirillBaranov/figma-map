import * as fs from "fs";
import * as os from "os";
import * as path from "path";

const MAX_LOG_BYTES = 20 * 1024 * 1024; // 20MB
const CHECK_INTERVAL_MS = 60_000;

// Must match bridgeLogPath() in internal/service/bridgeproc.go — this is
// the fallback used when FIGMA_MAP_BRIDGE_LOG isn't set, so rotation is on
// unconditionally rather than depending on how the process was launched.
export function defaultBridgeLogPath(): string {
  return path.join(os.homedir(), ".figma-map", "bridge.log");
}

// The bridge process is spawned detached with stdout/stderr redirected
// straight to this file's fd (see internal/service/bridgeproc.go) and can
// run for days. Nothing external ever rotates it, so it does its own
// copytruncate: copy current contents aside, then truncate the *same* file
// in place. Renaming would break this, since our stdout fd stays pointed at
// the renamed inode and future writes would never reach the new path.
export function startLogRotation(logPath: string): void {
  const timer = setInterval(() => {
    let size: number;
    try {
      size = fs.statSync(logPath).size;
    } catch {
      return; // log file not there (yet) — nothing to rotate
    }
    if (size < MAX_LOG_BYTES) return;
    try {
      fs.copyFileSync(logPath, `${logPath}.1`);
      fs.truncateSync(logPath, 0);
    } catch (err) {
      console.error("Log rotation failed:", err);
    }
  }, CHECK_INTERVAL_MS);
  timer.unref();
}

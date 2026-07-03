import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

/** Default on-disk home for everything the backend persists (per ADR-0003). */
export function defaultDataDir(): string {
  return join(homedir(), ".figma-map", "backend");
}

/** Reads a JSON file, returning `fallback` if it doesn't exist or fails to parse. */
export function readJSON<T>(path: string, fallback: T): T {
  if (!existsSync(path)) return fallback;
  try {
    return JSON.parse(readFileSync(path, "utf-8")) as T;
  } catch {
    return fallback;
  }
}

/**
 * Writes JSON atomically: write to a temp file, then rename over the target.
 * Avoids a partial/corrupt file if the process dies mid-write.
 */
export function writeJSONAtomic(path: string, data: unknown): void {
  const dir = dirnameOf(path);
  if (!existsSync(dir)) mkdirSync(dir, { recursive: true });
  const tmp = `${path}.tmp`;
  writeFileSync(tmp, JSON.stringify(data));
  renameSync(tmp, path);
}

function dirnameOf(path: string): string {
  const idx = path.lastIndexOf("/");
  return idx === -1 ? "." : path.slice(0, idx);
}

/** 7 days, per ADR-0003 §4 — the shared default retention window. */
export const TTL_MS = 7 * 24 * 60 * 60 * 1000;

export function isExpired(isoDate: string, now: number = Date.now()): boolean {
  return now - new Date(isoDate).getTime() > TTL_MS;
}

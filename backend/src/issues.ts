import { randomUUID } from "node:crypto";
import { join } from "node:path";
import type { FlaggedIssue, NewFlaggedIssue } from "./types.js";
import { defaultDataDir, isExpired, readJSON, writeJSONAtomic } from "./store.js";

export type { NewFlaggedIssue };

/**
 * Inbox for issues flagged by the browser extension. Persisted to
 * `<dataDir>/issues.json` (atomic write, load-on-construct) per ADR-0003 —
 * only the WebSocket bridge and the reload signal stay in-memory-only now.
 *
 * Entries expire after 7 days regardless of ack state (ADR-0003 §4): if a
 * week goes by with no reaction, the page or design has likely already
 * moved on, so pruning is unconditional, not just a fallback for unacked
 * ones the human forgot about.
 */
export class IssueStore {
  private issues: Map<string, FlaggedIssue>;
  private readonly path: string;

  constructor(dataDir: string = defaultDataDir()) {
    this.path = join(dataDir, "issues.json");
    const loaded = readJSON<FlaggedIssue[]>(this.path, []);
    this.issues = new Map(loaded.map((i) => [i.id, i]));
    this.pruneExpired();
  }

  add(issue: NewFlaggedIssue): FlaggedIssue {
    const full: FlaggedIssue = {
      ...issue,
      id: randomUUID(),
      createdAt: new Date().toISOString(),
    };
    this.issues.set(full.id, full);
    this.persist();
    return full;
  }

  list(fileKey?: string): FlaggedIssue[] {
    this.pruneExpired();
    const all = Array.from(this.issues.values());
    const filtered = fileKey ? all.filter((i) => i.fileKey === fileKey) : all;
    return filtered.sort((a, b) => a.createdAt.localeCompare(b.createdAt));
  }

  ack(id: string): boolean {
    const deleted = this.issues.delete(id);
    if (deleted) this.persist();
    return deleted;
  }

  private pruneExpired(): void {
    const now = Date.now();
    let changed = false;
    for (const [id, issue] of this.issues) {
      if (isExpired(issue.createdAt, now)) {
        this.issues.delete(id);
        changed = true;
      }
    }
    if (changed) this.persist();
  }

  private persist(): void {
    writeJSONAtomic(this.path, Array.from(this.issues.values()));
  }
}

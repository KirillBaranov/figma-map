import { randomUUID } from "node:crypto";
import type { FlaggedIssue } from "./types.js";

export type NewFlaggedIssue = Omit<FlaggedIssue, "id" | "createdAt">;

/**
 * In-memory inbox for issues flagged by the browser extension. Stateless by
 * design, like the rest of the bridge — issues do not survive a restart.
 */
export class IssueStore {
  private issues = new Map<string, FlaggedIssue>();

  add(issue: NewFlaggedIssue): FlaggedIssue {
    const full: FlaggedIssue = {
      ...issue,
      id: randomUUID(),
      createdAt: new Date().toISOString(),
    };
    this.issues.set(full.id, full);
    return full;
  }

  list(fileKey?: string): FlaggedIssue[] {
    const all = Array.from(this.issues.values());
    const filtered = fileKey ? all.filter((i) => i.fileKey === fileKey) : all;
    return filtered.sort((a, b) => a.createdAt.localeCompare(b.createdAt));
  }

  ack(id: string): boolean {
    return this.issues.delete(id);
  }
}

import { randomUUID } from "node:crypto";
import type { CompareHistoryEntry, CompareSession } from "./types.js";

const HISTORY_CAP = 10;

/**
 * In-memory holder for the overlay-compare session. Stateless by design,
 * like the rest of the bridge — a session does not survive a restart. Single
 * slot, not a list: there is one active comparison at a time, mirroring what
 * chrome.storage.local held before this moved server-side.
 *
 * Alongside it, a small history of past sessions (pushed only when a new
 * reference image loads, not on every tweak to the active one) — capped at
 * HISTORY_CAP unpinned entries, FIFO-evicted; pinned entries are exempt.
 */
export class CompareSessionStore {
  private session: CompareSession | null = null;
  private history: CompareHistoryEntry[] = [];

  get(): CompareSession | null {
    return this.session;
  }

  set(session: CompareSession): CompareSession {
    this.session = session;
    return this.session;
  }

  clear(): void {
    this.session = null;
  }

  pushHistory(session: CompareSession): CompareHistoryEntry {
    const entry: CompareHistoryEntry = {
      ...session,
      id: randomUUID(),
      createdAt: new Date().toISOString(),
      pinned: false,
    };
    this.history.unshift(entry);
    this.evictOverflow();
    return entry;
  }

  listHistory(): CompareHistoryEntry[] {
    return this.history;
  }

  setPinned(id: string, pinned: boolean): CompareHistoryEntry | null {
    const entry = this.history.find((e) => e.id === id);
    if (!entry) return null;
    entry.pinned = pinned;
    return entry;
  }

  deleteHistory(id: string): boolean {
    const before = this.history.length;
    this.history = this.history.filter((e) => e.id !== id);
    return this.history.length < before;
  }

  private evictOverflow(): void {
    const unpinned = this.history.filter((e) => !e.pinned);
    if (unpinned.length <= HISTORY_CAP) return;
    const dropIds = new Set(
      unpinned.slice(HISTORY_CAP).map((e) => e.id)
    );
    this.history = this.history.filter((e) => !dropIds.has(e.id));
  }
}

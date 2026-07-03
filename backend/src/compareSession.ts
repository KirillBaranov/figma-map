import { randomUUID } from "node:crypto";
import { join } from "node:path";
import type { CompareHistoryEntry, CompareSession } from "./types.js";
import { defaultDataDir, isExpired, readJSON, writeJSONAtomic } from "./store.js";

const HISTORY_CAP = 10;

interface PersistedState {
  session: CompareSession | null;
  history: CompareHistoryEntry[];
}

/**
 * Holder for the overlay-compare session, persisted to
 * `<dataDir>/compare-session.json` (atomic write, load-on-construct) per
 * ADR-0003 — single slot, not a list: there is one active comparison at a
 * time, mirroring what chrome.storage.local held before this moved
 * server-side.
 *
 * Alongside it, a small history of past sessions (pushed only when a new
 * reference image loads, not on every tweak to the active one).
 *
 * Retention (ADR-0003 §4): both the active slot and unpinned history entries
 * expire after 7 days of inactivity — imported Figma templates/screenshots
 * the user wants to keep are marked `pinned` (by the extension, at import
 * time) and are exempt from TTL and from the FIFO cap, removed only by
 * explicit user action.
 */
export class CompareSessionStore {
  private session: CompareSession | null;
  private history: CompareHistoryEntry[];
  private readonly path: string;

  constructor(dataDir: string = defaultDataDir()) {
    this.path = join(dataDir, "compare-session.json");
    const loaded = readJSON<PersistedState>(this.path, { session: null, history: [] });
    this.session = loaded.session;
    this.history = loaded.history;
    this.pruneExpired();
  }

  get(): CompareSession | null {
    this.pruneExpired();
    return this.session;
  }

  set(session: CompareSession): CompareSession {
    this.session = { ...session, updatedAt: new Date().toISOString() };
    this.persist();
    return this.session;
  }

  clear(): void {
    this.session = null;
    this.persist();
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
    this.persist();
    return entry;
  }

  listHistory(): CompareHistoryEntry[] {
    this.pruneExpired();
    return this.history;
  }

  setPinned(id: string, pinned: boolean): CompareHistoryEntry | null {
    const entry = this.history.find((e) => e.id === id);
    if (!entry) return null;
    entry.pinned = pinned;
    this.persist();
    return entry;
  }

  deleteHistory(id: string): boolean {
    const before = this.history.length;
    this.history = this.history.filter((e) => e.id !== id);
    const deleted = this.history.length < before;
    if (deleted) this.persist();
    return deleted;
  }

  private pruneExpired(): void {
    const now = Date.now();
    let changed = false;

    if (this.session && isExpired(this.session.updatedAt ?? new Date(0).toISOString(), now)) {
      this.session = null;
      changed = true;
    }

    const before = this.history.length;
    this.history = this.history.filter((e) => e.pinned || !isExpired(e.createdAt, now));
    if (this.history.length !== before) changed = true;

    if (this.evictOverflow()) changed = true;

    if (changed) this.persist();
  }

  /** Returns true if anything was dropped. */
  private evictOverflow(): boolean {
    const unpinned = this.history.filter((e) => !e.pinned);
    if (unpinned.length <= HISTORY_CAP) return false;
    const dropIds = new Set(unpinned.slice(HISTORY_CAP).map((e) => e.id));
    this.history = this.history.filter((e) => !dropIds.has(e.id));
    return true;
  }

  private persist(): void {
    writeJSONAtomic(this.path, { session: this.session, history: this.history });
  }
}

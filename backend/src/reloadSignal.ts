/**
 * One-shot dev signal: an agent/CLI requests a reload, the extension's own
 * polling consumes it (clearing the flag) and calls chrome.runtime.reload()
 * on itself. Stateless by design like the rest of the bridge — a pending
 * request does not survive a restart, same as issues/compare-session.
 */
export class ReloadSignal {
  private requested = false;

  request(): void {
    this.requested = true;
  }

  /** Consumes the pending flag — returns true at most once per request(). */
  consume(): boolean {
    const was = this.requested;
    this.requested = false;
    return was;
  }
}

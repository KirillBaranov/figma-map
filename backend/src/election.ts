import type { Node } from "./node.js";
import { Role } from "./types.js";

const HEALTH_CHECK_TIMEOUT_MS = 2_000;
const MIN_POLL_INTERVAL_MS = 3_000;
const POLL_JITTER_MS = 2_000;

/**
 * Drives role transitions for a single process: claim the port as leader if
 * free, otherwise follow whoever holds it. A periodic health check watches
 * the leader (from a follower's perspective) and triggers takeover once it
 * stops answering /ping.
 */
export class Election {
  private readonly leaderHealthUrl: string;
  private pollTimer: ReturnType<typeof setInterval> | null = null;

  constructor(
    private readonly port: number,
    private readonly node: Node
  ) {
    this.leaderHealthUrl = `http://localhost:${port}/ping`;
  }

  async start(): Promise<void> {
    await this.resolveInitialRole();

    // Jitter the poll cadence so multiple followers don't all probe /ping
    // in lockstep after a leader dies.
    const cadence = MIN_POLL_INTERVAL_MS + Math.random() * POLL_JITTER_MS;
    this.pollTimer = setInterval(() => {
      this.onPollTick().catch((err) => {
        console.error("Election check error:", err);
      });
    }, cadence);
  }

  stop(): void {
    if (this.pollTimer) {
      clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }

  private async onPollTick(): Promise<void> {
    if (this.node.role === Role.Leader) {
      return; // we hold the port, nothing to watch
    }
    if (this.node.role === Role.Unknown) {
      await this.resolveInitialRole();
      return;
    }
    // Role.Follower
    const leaderReachable = await this.pingLeader();
    if (leaderReachable) return;

    console.error("Leader not responding, attempting takeover...");
    try {
      await this.node.becomeLeader();
    } catch (err) {
      console.error("Failed to become leader:", err);
    }
  }

  private async resolveInitialRole(): Promise<void> {
    const claimed = await this.tryClaimLeadership();
    if (claimed) return;

    // Someone else already owns the port — follow them if they're healthy.
    // If the ping also fails, we stay Unknown and retry on the next tick.
    if (await this.pingLeader()) {
      this.node.becomeFollower();
    }
  }

  private async tryClaimLeadership(): Promise<boolean> {
    try {
      await this.node.becomeLeader();
      return true;
    } catch {
      return false;
    }
  }

  private async pingLeader(): Promise<boolean> {
    try {
      const res = await fetch(this.leaderHealthUrl, {
        signal: AbortSignal.timeout(HEALTH_CHECK_TIMEOUT_MS),
      });
      return res.ok;
    } catch {
      return false;
    }
  }
}

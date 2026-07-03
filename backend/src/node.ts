import { Leader } from "./leader.js";
import { Follower } from "./follower.js";
import { Role } from "./types.js";
import type { BridgeResponse, ConnectedFile } from "./types.js";

type NodeState =
  | { role: Role.Unknown }
  | { role: Role.Leader; leader: Leader }
  | { role: Role.Follower };

/**
 * Single entry point tool handlers call into. Depending on which role this
 * process currently holds, requests are served locally against the Figma
 * plugin bridge (leader) or forwarded over HTTP to whoever holds the port
 * (follower). Role transitions are driven externally by Election.
 */
export class Node {
  private state: NodeState = { role: Role.Unknown };
  private readonly follower: Follower;

  constructor(private readonly port: number) {
    this.follower = new Follower(`http://localhost:${port}`);
  }

  get role(): Role {
    return this.state.role;
  }

  get roleName(): string {
    switch (this.state.role) {
      case Role.Leader:
        return "LEADER";
      case Role.Follower:
        return "FOLLOWER";
      default:
        return "UNKNOWN";
    }
  }

  send(requestType: string, nodeIds?: string[], fileKey?: string): Promise<BridgeResponse> {
    return this.sendWithParams(requestType, nodeIds, undefined, fileKey);
  }

  sendWithParams(
    requestType: string,
    nodeIds?: string[],
    params?: Record<string, unknown>,
    fileKey?: string
  ): Promise<BridgeResponse> {
    if (this.state.role === Role.Leader) {
      return this.state.leader
        .getBridge()
        .sendWithParams(requestType, nodeIds, params, fileKey);
    }
    return this.follower.sendWithParams(requestType, nodeIds, params, fileKey);
  }

  listConnectedFiles(): ConnectedFile[] | undefined {
    if (this.state.role === Role.Leader) {
      return this.state.leader.getBridge().listConnectedFiles();
    }
    // Followers return undefined — the tool handler falls back to RPC.
    return undefined;
  }

  async becomeLeader(): Promise<void> {
    if (this.state.role === Role.Leader) return;

    const leader = new Leader(this.port);
    await leader.start();

    this.state = { role: Role.Leader, leader };
    console.error("Became LEADER");
  }

  becomeFollower(): void {
    if (this.state.role === Role.Follower) return;

    if (this.state.role === Role.Leader) {
      this.state.leader.stop();
    }

    this.state = { role: Role.Follower };
    console.error("Became FOLLOWER");
  }

  stop(): void {
    if (this.state.role === Role.Leader) {
      this.state.leader.stop();
    }
    this.state = { role: Role.Unknown };
  }
}

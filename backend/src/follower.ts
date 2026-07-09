import type { BridgeResponse, ConnectedFile, RPCRequest, RPCResponse } from "./types.js";

/** Must match the `API` prefix the leader's HTTP server listens on (see leader.ts). */
const API_PREFIX = "/api/v1";

// Generous stopgap ceiling: the leader's own bridge now uses a sliding
// inactivity timeout that can legitimately run past the old flat 35s for a
// large-but-progressing selection. This doesn't make the Follower->Leader
// hop itself progress-aware (that would need a streaming RPC), it just
// stops it from cutting the request off before the leader's diagnosis would.
const RPC_TIMEOUT_MS = 180_000;
const LIST_FILES_TIMEOUT_MS = 5_000;
const PING_TIMEOUT_MS = 2_000;

/**
 * Speaks for a follower process: every tool call is shipped to whichever
 * process currently holds the leader port, over its HTTP RPC endpoint.
 */
export class Follower {
  constructor(private readonly leaderBaseUrl: string) {}

  send(requestType: string, nodeIds?: string[], fileKey?: string): Promise<BridgeResponse> {
    return this.sendWithParams(requestType, nodeIds, undefined, fileKey);
  }

  async sendWithParams(
    requestType: string,
    nodeIds?: string[],
    params?: Record<string, unknown>,
    fileKey?: string
  ): Promise<BridgeResponse> {
    const payload: RPCRequest = { tool: requestType };
    if (nodeIds && nodeIds.length > 0) payload.nodeIds = nodeIds;
    if (params && Object.keys(params).length > 0) payload.params = params;
    if (fileKey) payload.fileKey = fileKey;

    const data = await this.postRpc(payload, RPC_TIMEOUT_MS);
    return { type: requestType, requestId: "", data };
  }

  async listConnectedFiles(): Promise<ConnectedFile[]> {
    const data = await this.postRpc({ tool: "list_files" }, LIST_FILES_TIMEOUT_MS);
    return (data as ConnectedFile[]) ?? [];
  }

  async ping(): Promise<boolean> {
    try {
      const res = await fetch(`${this.leaderBaseUrl}/ping`, {
        signal: AbortSignal.timeout(PING_TIMEOUT_MS),
      });
      return res.ok;
    } catch {
      return false;
    }
  }

  private async postRpc(payload: RPCRequest, timeoutMs: number): Promise<unknown> {
    const res = await fetch(`${this.leaderBaseUrl}${API_PREFIX}/rpc`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      signal: AbortSignal.timeout(timeoutMs),
    });

    if (!res.ok) {
      throw new Error(`Leader returned status ${res.status}`);
    }

    const parsed = (await res.json()) as RPCResponse;
    if (parsed.error) {
      throw new Error(parsed.error);
    }
    return parsed.data;
  }
}

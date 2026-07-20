import { WebSocketServer, WebSocket } from "ws";
import type { IncomingMessage } from "node:http";
import type { Duplex } from "node:stream";
import type { BridgeRequest, BridgeResponse, ConnectedFile } from "./types.js";

// Grace period to hear an ack before assuming the plugin itself is
// unresponsive (unfocused/suspended by Figma). Resets on any activity.
const ACK_GRACE_MS = 5_000;
// Inactivity window once acked — reset on every progress tick, so a
// legitimately long-running (but progressing) request never times out
// just for being large. Generous on purpose: a whole-page get_selection can
// legitimately run past a minute (thousands of nodes, each round-tripping
// through Figma's own internal plugin API), and the plugin keeps sending a
// heartbeat every few seconds the whole time it's still actually working —
// so this window is really "how long can the plugin go silent", not "how
// long can the whole request take".
const INACTIVITY_TIMEOUT_MS = 60_000;
// Independent ceiling on how long real progress (an actual "chunk", i.e.
// data) can stop arriving, checked by a periodic watchdog rather than the
// per-message sliding timer above. The generic dispatcher heartbeat
// (kind: "progress", no payload) fires every 4s purely to prove the plugin
// process itself is alive, and resets the sliding timer just like a chunk
// does — which means a request whose actual work has genuinely stalled
// (confirmed live: 8+ minutes with a heartbeat every 4s and zero new
// chunks) would otherwise never time out, since the heartbeat keeps
// resetting the same clock the real signal does. This ceiling is measured
// from the last *chunk*, independent of how recently a bare heartbeat
// arrived.
const CHUNK_STALL_TIMEOUT_MS = 90_000;
const STALL_WATCHDOG_INTERVAL_MS = 10_000;
// How long a request can go without real progress (a chunk) before the
// connection is reported "dormant" rather than "connected" — well short of
// CHUNK_STALL_TIMEOUT_MS's hard reject, so an agent calling list_files (or
// doctor) mid-request sees an honest "the tab looks backgrounded" instead of
// silently waiting the full 90s alongside the stuck request itself.
const DORMANT_THRESHOLD_MS = 15_000;

// One node of the reassembly tree for a "chunk"-streamed result, addressed
// purely structurally by path — see BridgeResponse's `path`/`containerType`/
// `count` docs in types.ts. Built up lazily as chunks arrive, walked once on
// "final" to produce the real `data` value.
interface ChunkNode {
  data?: unknown;
  containerType?: "object" | "array";
  count?: number;
  children: Map<number, ChunkNode>;
}

function getOrCreateChunkNode(root: ChunkNode, path: number[]): ChunkNode {
  let node = root;
  for (const segment of path) {
    let child = node.children.get(segment);
    if (!child) {
      child = { children: new Map() };
      node.children.set(segment, child);
    }
    node = child;
  }
  return node;
}

function assembleChunkNode(node: ChunkNode): unknown {
  if (!node.containerType) return node.data;
  const items: unknown[] = [];
  for (let i = 0; i < (node.count ?? 0); i++) {
    const child = node.children.get(i);
    items.push(child ? assembleChunkNode(child) : undefined);
  }
  return node.containerType === "array" ? items : { ...(node.data as object), children: items };
}

interface PendingRequest {
  resolve: (resp: BridgeResponse) => void;
  reject: (err: Error) => void;
  timer: ReturnType<typeof setTimeout>;
  ws: WebSocket;
  fileKey: string;
  startedAt: number;
  acked: boolean;
  lastProgress?: { done: number; total: number };
  requestType: string;
  request: BridgeRequest;
  retried: boolean;
  chunkRoot?: ChunkNode;
  // Set on send, updated only by real "chunk" messages — see
  // CHUNK_STALL_TIMEOUT_MS above. Deliberately not touched by the bare
  // heartbeat, unlike the sliding `timer`.
  lastChunkAt: number;
}

interface ConnectionEntry {
  ws: WebSocket;
  fileKey: string;
  fileName: string;
  // "dormant" means a request against this connection is going without real
  // progress for DORMANT_THRESHOLD_MS — inferred from request behavior (see
  // checkStalledRequests), not a true ambient probe: there's no standing
  // heartbeat independent of an in-flight request today. Reset to
  // "connected" the moment any chunk (real progress) arrives.
  status: "connected" | "dormant";
}

export class Bridge {
  private wss: WebSocketServer;
  private connections = new Map<string, ConnectionEntry>();
  private pending = new Map<string, PendingRequest>();
  private counter = 0;
  private stallWatchdog: ReturnType<typeof setInterval>;

  constructor() {
    this.wss = new WebSocketServer({ noServer: true });
    this.stallWatchdog = setInterval(() => this.checkStalledRequests(), STALL_WATCHDOG_INTERVAL_MS);
    this.stallWatchdog.unref();
  }

  // Runs independently of any one request's sliding timer — see
  // CHUNK_STALL_TIMEOUT_MS's comment for why the sliding timer alone can't
  // catch this: a bare heartbeat keeps resetting it even when real chunk
  // progress has genuinely stopped.
  private checkStalledRequests(): void {
    const now = Date.now();
    for (const pending of this.pending.values()) {
      // Dormant: acked (the plugin process is alive) but no real progress
      // (a chunk) for DORMANT_THRESHOLD_MS. Deliberately not gated on
      // pending.chunkRoot — a non-streamed request (e.g. get_node) never
      // sends a "chunk" message at all, only heartbeats, so lastChunkAt
      // stays at its initial startedAt value and this still fires. This is
      // a status flip only, not a reject — the hard CHUNK_STALL_TIMEOUT_MS
      // reject below (chunked requests only) and the sliding
      // INACTIVITY_TIMEOUT_MS/ACK_GRACE_MS timers elsewhere still own
      // actually failing the request.
      if (pending.acked && now - pending.lastChunkAt > DORMANT_THRESHOLD_MS) {
        const entry = this.connections.get(pending.fileKey);
        if (entry) entry.status = "dormant";
      }
    }
    for (const [requestId, pending] of this.pending) {
      if (pending.chunkRoot && now - pending.lastChunkAt > CHUNK_STALL_TIMEOUT_MS) {
        clearTimeout(pending.timer);
        this.pending.delete(requestId);
        const elapsed = now - pending.startedAt;
        const stalledFor = now - pending.lastChunkAt;
        console.error(
          `✗ ${requestId} ${pending.requestType} stalled after ${elapsed}ms (no new chunk for ${stalledFor}ms despite live heartbeat)`
        );
        pending.reject(
          new Error(
            `${pending.requestType} (${requestId}) stalled: no progress for ${Math.round(stalledFor / 1000)}s despite the plugin still reporting alive — likely Figma itself pausing the plugin (e.g. window backgrounded) or the request should be scoped smaller`
          )
        );
      }
    }
  }

  handleUpgrade(request: IncomingMessage, socket: Duplex, head: Buffer): void {
    if (request.url == undefined) {
      console.error("Plugin connected without url, rejecting");
      socket.destroy();
      return;
    }

    const url = new URL(request.url, "http://localhost");
    const { fileKey, fileName = "Unknown" } = Object.fromEntries(
      url.searchParams
    );

    if (!fileKey) {
      console.error("Plugin connected without fileKey, rejecting");
      socket.destroy();
      return;
    }

    this.wss.handleUpgrade(request, socket, head, (ws) => {
      this.handleConnection(ws, fileKey, fileName);
    });
  }

  private handleConnection(
    ws: WebSocket,
    fileKey: string,
    fileName: string
  ): void {
    // Replace existing connection for the same file
    const existing = this.connections.get(fileKey);
    if (existing) {
      existing.ws.close();
    }
    this.connections.set(fileKey, { ws, fileKey, fileName, status: "connected" });
    console.error(`Plugin connected: ${fileName} (${fileKey})`);

    ws.on("message", (data) => {
      try {
        const resp: BridgeResponse = JSON.parse(data.toString());
        const pending = this.pending.get(resp.requestId);
        if (!pending) {
          console.error(`Response for unknown/expired request ${resp.requestId}`);
          return;
        }

        if (resp.kind === "ack") {
          pending.acked = true;
          console.error(`… ${resp.requestId} ${resp.type} acked`);
          this.armTimer(resp.requestId, pending);
          return;
        }

        if (resp.kind === "progress") {
          pending.acked = true;
          pending.lastProgress =
            resp.done != undefined && resp.total != undefined
              ? { done: resp.done, total: resp.total }
              : pending.lastProgress;
          console.error(
            `… ${resp.requestId} ${resp.type} progress ${resp.done ?? "?"}/${resp.total ?? "?"}`
          );
          this.armTimer(resp.requestId, pending);
          return;
        }

        if (resp.kind === "chunk") {
          pending.acked = true;
          pending.lastChunkAt = Date.now();
          const entry = this.connections.get(fileKey);
          if (entry) entry.status = "connected"; // real progress → not dormant
          if (!pending.chunkRoot) pending.chunkRoot = { children: new Map() };
          const node = getOrCreateChunkNode(pending.chunkRoot, resp.path ?? []);
          node.data = resp.data;
          node.containerType = resp.containerType;
          node.count = resp.count;
          console.error(
            `… ${resp.requestId} ${resp.type} chunk [${(resp.path ?? []).join(",")}]${resp.containerType ? ` (${resp.containerType}, ${resp.count})` : ""}`
          );
          this.armTimer(resp.requestId, pending);
          return;
        }

        clearTimeout(pending.timer);
        this.pending.delete(resp.requestId);
        if (!resp.error) {
          const entry = this.connections.get(fileKey);
          if (entry) entry.status = "connected"; // completed → not dormant
        }
        const elapsed = Date.now() - pending.startedAt;
        if (resp.error) {
          console.error(
            `✗ ${resp.requestId} ${resp.type} failed after ${elapsed}ms: ${resp.error}`
          );
          pending.resolve(resp);
        } else if (pending.chunkRoot) {
          console.error(`✓ ${resp.requestId} ${resp.type} (${elapsed}ms, chunked)`);
          pending.resolve({ ...resp, data: assembleChunkNode(pending.chunkRoot) });
        } else {
          console.error(`✓ ${resp.requestId} ${resp.type} (${elapsed}ms)`);
          pending.resolve(resp);
        }
      } catch {
        console.error("Invalid response from plugin");
      }
    });

    ws.on("close", () => {
      const current = this.connections.get(fileKey);
      if (current?.ws === ws) {
        this.connections.delete(fileKey);
        console.error(`Plugin disconnected: ${fileName} (${fileKey})`);
      }
      this.rejectPendingForSocket(
        ws,
        `Plugin disconnected: ${fileName} (${fileKey})`,
        fileKey
      );
    });

    ws.on("error", (err) => {
      console.error("WebSocket error:", err.message);
      const current = this.connections.get(fileKey);
      if (current?.ws === ws) {
        this.connections.delete(fileKey);
      }
      this.rejectPendingForSocket(
        ws,
        `Plugin connection error (${fileName}): ${err.message}`,
        fileKey
      );
    });
  }

  // fileKey, when given, lets a fast reconnect resume in-flight requests
  // instead of failing them: handleConnection closes the old socket and
  // registers the new one *before* the old socket's close event fires here,
  // so if this.connections.get(fileKey) already points elsewhere, that's a
  // replacement plugin instance for the same file, not really "gone" — the
  // agent's original call keeps waiting (a bit longer), it doesn't need to
  // retry itself. Falls back to rejecting when there's no replacement (a
  // genuine disconnect) — unchanged from before.
  private rejectPendingForSocket(ws: WebSocket, reason: string, fileKey?: string): void {
    const replacement = fileKey ? this.connections.get(fileKey) : undefined;
    const resumable = replacement && replacement.ws !== ws && replacement.ws.readyState === WebSocket.OPEN;

    for (const [id, p] of this.pending) {
      if (p.ws !== ws) continue;
      if (resumable) {
        console.error(`↻ ${id} resuming on reconnected plugin (${fileKey})`);
        p.ws = replacement.ws;
        p.acked = false;
        p.lastChunkAt = Date.now();
        this.armTimer(id, p);
        replacement.ws.send(JSON.stringify(p.request), (err) => {
          if (err) {
            clearTimeout(p.timer);
            this.pending.delete(id);
            console.error(`✗ ${id} resume failed: ${err.message}`);
            p.reject(err);
          }
        });
        continue;
      }
      clearTimeout(p.timer);
      this.pending.delete(id);
      console.error(`✗ ${id} dropped: ${reason}`);
      p.reject(new Error(reason));
    }
  }

  /** (Re)arms a pending request's inactivity timer from "now", using a
   * shorter window pre-ack (bare silence from the plugin) and a longer one
   * once it's confirmed alive and working. */
  private armTimer(requestId: string, pending: PendingRequest): void {
    clearTimeout(pending.timer);
    const windowMs = pending.acked ? INACTIVITY_TIMEOUT_MS : ACK_GRACE_MS;
    pending.timer = setTimeout(() => this.handleTimeout(requestId), windowMs);
  }

  private handleTimeout(requestId: string): void {
    const pending = this.pending.get(requestId);
    if (!pending) return;

    const elapsed = Date.now() - pending.startedAt;
    const { requestType, request, ws, acked, lastProgress, retried } = pending;

    // Message never acked and connection is still open: most likely lost
    // in transit (e.g. mid-reconnect) rather than the plugin being stuck on
    // it. Safe to retry once — the plugin dedupes by requestId, so even if
    // the original *did* arrive and just lost its ack, resending is a no-op
    // there rather than a double-execution.
    if (!acked && !retried && ws.readyState === WebSocket.OPEN) {
      console.error(`… ${requestId} ${requestType} no ack after ${elapsed}ms, retrying once`);
      pending.retried = true;
      pending.startedAt = Date.now();
      this.armTimer(requestId, pending);
      ws.send(JSON.stringify(request), (err) => {
        if (err) {
          this.pending.delete(requestId);
          clearTimeout(pending.timer);
          console.error(`✗ ${requestId} ${requestType} retry send failed: ${err.message}`);
          pending.reject(err);
        }
      });
      return;
    }

    this.pending.delete(requestId);
    const diagnosis = !acked
      ? "plugin unresponsive (no ack received — window may be unfocused or suspended by Figma)"
      : lastProgress
        ? `stuck after ${lastProgress.done}/${lastProgress.total} — handler stopped reporting progress`
        : "acked but handler hung with no progress";
    console.error(`✗ ${requestId} ${requestType} timed out after ${elapsed}ms: ${diagnosis}`);
    // Best-effort: tell the plugin to abandon the work rather than let it
    // keep grinding away (and competing with future requests) for a result
    // nobody's waiting on anymore.
    if (ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: "cancel", requestId }));
    }
    pending.reject(new Error(`${requestType} (${requestId}) timed out: ${diagnosis}`));
  }

  /**
   * Resolve which connection to use.
   * - If fileKey is provided, use that specific connection.
   * - If only one file is connected and no fileKey given, use it (backward compat).
   * - If multiple files connected and no fileKey, throw with a helpful message.
   */
  private resolveConnection(fileKey?: string): ConnectionEntry {
    if (fileKey) {
      const entry = this.connections.get(fileKey);
      if (!entry) {
        const available = this.listConnectedFiles();
        const hint =
          available.length > 0
            ? ` Connected files: ${available.map((f) => `"${f.fileName}" (fileKey: ${f.fileKey})`).join(", ")}`
            : " No files are currently connected.";
        throw new Error(`No plugin connected for fileKey "${fileKey}".${hint}`);
      }
      return entry;
    }

    if (this.connections.size === 0) {
      throw new Error(
        "No plugin connected. Open a Figma file and run the bridge plugin."
      );
    }

    if (this.connections.size === 1) {
      return this.connections.values().next().value!;
    }

    const files = this.listConnectedFiles();
    throw new Error(
      `Multiple files connected. Specify a fileKey to choose which file to query. Connected files: ${files.map((f) => `"${f.fileName}" (fileKey: ${f.fileKey})`).join(", ")}. Use the list_files tool to see all connected files.`
    );
  }

  listConnectedFiles(): ConnectedFile[] {
    return [...this.connections.values()].map((entry) => ({
      fileKey: entry.fileKey,
      fileName: entry.fileName,
      status: entry.status,
    }));
  }

  send(
    requestType: string,
    nodeIds?: string[],
    fileKey?: string
  ): Promise<BridgeResponse> {
    return this.sendWithParams(requestType, nodeIds, undefined, fileKey);
  }

  sendWithParams(
    requestType: string,
    nodeIds?: string[],
    params?: Record<string, unknown>,
    fileKey?: string
  ): Promise<BridgeResponse> {
    return new Promise((resolve, reject) => {
      let entry: ConnectionEntry;
      try {
        entry = this.resolveConnection(fileKey);
      } catch (err) {
        reject(err);
        return;
      }
      const conn = entry.ws;
      const resolvedFileKey = entry.fileKey;

      if (conn.readyState !== WebSocket.OPEN) {
        reject(new Error("Plugin not connected"));
        return;
      }

      const requestId = this.nextId();
      const request: BridgeRequest = {
        type: requestType,
        requestId,
      };
      if (nodeIds && nodeIds.length > 0) {
        request.nodeIds = nodeIds;
      }
      if (params && Object.keys(params).length > 0) {
        request.params = params;
      }

      const startedAt = Date.now();
      const target = nodeIds?.length ? ` nodeIds=${nodeIds.join(",")}` : "";
      const withParams = params ? ` params=${JSON.stringify(params)}` : "";
      console.error(`→ ${requestId} ${requestType}${target}${withParams}`);

      const pending: PendingRequest = {
        resolve,
        reject,
        timer: setTimeout(() => this.handleTimeout(requestId), ACK_GRACE_MS),
        ws: conn,
        fileKey: resolvedFileKey,
        startedAt,
        acked: false,
        requestType,
        request,
        retried: false,
        lastChunkAt: startedAt,
      };
      this.pending.set(requestId, pending);

      conn.send(JSON.stringify(request), (err) => {
        if (err) {
          clearTimeout(pending.timer);
          this.pending.delete(requestId);
          console.error(`✗ ${requestId} ${requestType} send failed: ${err.message}`);
          reject(err);
        }
      });
    });
  }

  private nextId(): string {
    this.counter++;
    const now = new Date();
    const hh = String(now.getHours()).padStart(2, "0");
    const mm = String(now.getMinutes()).padStart(2, "0");
    const ss = String(now.getSeconds()).padStart(2, "0");
    return `req-${hh}${mm}${ss}-${this.counter}`;
  }

  close(): void {
    clearInterval(this.stallWatchdog);
    // Reject all pending requests
    for (const [id, { reject, timer }] of this.pending) {
      clearTimeout(timer);
      reject(new Error("Bridge closed"));
    }
    this.pending.clear();

    for (const [, entry] of this.connections) {
      entry.ws.close();
    }
    this.connections.clear();
    this.wss.close();
  }
}

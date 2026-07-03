import http from "node:http";
import type { Duplex } from "node:stream";
import { Bridge } from "./bridge.js";
import { CompareSessionStore } from "./compareSession.js";
import { IssueStore } from "./issues.js";
import { ReloadSignal } from "./reloadSignal.js";
import { validateRpc } from "./schema.js";
import { executeSaveScreenshots } from "./tools.js";
import type { ExportFormat } from "./tools.js";
import {
  CompareSessionSchema,
  NewFlaggedIssueSchema,
  RPCRequestSchema,
  type RPCResponse,
} from "./types.js";
import { z } from "zod";
import { VERSION } from "./version.js";

/**
 * All versioned data-plane endpoints live under this prefix (ADR-0003 §2).
 * `/ping` and `/extension/reload` are deliberately left outside it — they're
 * infrastructure (health check, one-shot dev signal), not the data API.
 */
const API = "/api/v1";

/**
 * Leader owns the WebSocket bridge to Figma and exposes HTTP endpoints for followers.
 * Endpoints:
 *   /ws                             — WebSocket upgrade for the Figma plugin
 *   /ping                           — Health check (unversioned, infrastructure)
 *   /api/v1/rpc                     — JSON RPC for follower tool calls
 *   /api/v1/issues                  — Inbox for issues flagged by the browser extension
 *   /api/v1/compare-session         — Single source of truth for the extension's overlay-compare state
 *   /api/v1/compare-session/history — Past compare sessions (auto + pinned), for re-activation
 *   /extension/reload               — One-shot dev signal (unversioned, infrastructure): an
 *                                     agent/CLI POSTs, the extension's own polling picks it up
 *                                     and calls chrome.runtime.reload()
 */
export class Leader {
  private bridge: Bridge;
  private issues: IssueStore;
  private compareSession: CompareSessionStore;
  private reloadSignal: ReloadSignal;
  private server: http.Server | null = null;

  constructor(private port: number) {
    this.bridge = new Bridge();
    this.issues = new IssueStore();
    this.compareSession = new CompareSessionStore();
    this.reloadSignal = new ReloadSignal();
  }

  getBridge(): Bridge {
    return this.bridge;
  }

  start(): Promise<void> {
    return new Promise((resolve, reject) => {
      const server = http.createServer((req, res) => {
        if (req.url === "/ping" && req.method === "GET") {
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ status: "ok", version: VERSION }));
          return;
        }

        if (req.url === `${API}/rpc` && req.method === "POST") {
          this.handleRPC(req, res);
          return;
        }

        if (req.url === `${API}/issues` && req.method === "POST") {
          this.handleCreateIssue(req, res);
          return;
        }

        if (req.url?.startsWith(`${API}/issues`) && req.method === "GET") {
          this.handleListIssues(req, res);
          return;
        }

        if (req.url?.startsWith(`${API}/issues/`) && req.method === "DELETE") {
          this.handleAckIssue(req, res);
          return;
        }

        if (req.url === `${API}/compare-session` && req.method === "GET") {
          this.handleGetCompareSession(req, res);
          return;
        }

        if (req.url === `${API}/compare-session` && req.method === "PUT") {
          this.handleSaveCompareSession(req, res);
          return;
        }

        if (req.url === `${API}/compare-session` && req.method === "DELETE") {
          this.handleClearCompareSession(req, res);
          return;
        }

        if (req.url === `${API}/compare-session/history` && req.method === "GET") {
          this.handleListCompareHistory(req, res);
          return;
        }

        if (req.url === `${API}/compare-session/history` && req.method === "POST") {
          this.handlePushCompareHistory(req, res);
          return;
        }

        if (
          req.url?.startsWith(`${API}/compare-session/history/`) &&
          req.url.endsWith("/pin") &&
          req.method === "PUT"
        ) {
          this.handlePinCompareHistory(req, res);
          return;
        }

        if (
          req.url?.startsWith(`${API}/compare-session/history/`) &&
          req.method === "DELETE"
        ) {
          this.handleDeleteCompareHistory(req, res);
          return;
        }

        if (req.url === "/extension/reload" && req.method === "POST") {
          this.handleRequestReload(req, res);
          return;
        }

        if (req.url === "/extension/reload" && req.method === "GET") {
          this.handleConsumeReload(req, res);
          return;
        }

        res.writeHead(404);
        res.end("Not found");
      });

      server.on(
        "upgrade",
        (req: http.IncomingMessage, socket: Duplex, head: Buffer) => {
          const pathname = new URL(req.url ?? "", "http://localhost").pathname;
          if (pathname === "/ws") {
            this.bridge.handleUpgrade(req, socket, head);
          } else {
            socket.destroy();
          }
        }
      );

      // Fail fast if port is already in use
      server.once("error", (err: NodeJS.ErrnoException) => {
        reject(
          err.code === "EADDRINUSE"
            ? new Error(`Port ${this.port} already in use`)
            : err
        );
      });

      server.listen(this.port, () => {
        this.server = server;
        console.error(`Leader listening on :${this.port}`);
        resolve();
      });
    });
  }

  private handleRPC(req: http.IncomingMessage, res: http.ServerResponse): void {
    let body = "";
    req.on("data", (chunk: Buffer) => {
      body += chunk.toString();
    });
    req.on("end", async () => {
      try {
        const parsed = RPCRequestSchema.safeParse(JSON.parse(body));
        if (!parsed.success) {
          this.sendJSON(res, 400, { error: parsed.error.issues[0].message });
          return;
        }
        const rpcReq = parsed.data;

        // Handle list_files as a special RPC (not forwarded to plugin)
        if (rpcReq.tool === "list_files") {
          this.sendJSON(res, 200, {
            data: this.bridge.listConnectedFiles(),
          });
          return;
        }

        const validationError = validateRpc(
          rpcReq.tool,
          rpcReq.nodeIds,
          rpcReq.params
        );
        if (validationError) {
          this.sendJSON(res, 400, { error: validationError });
          return;
        }

        const fileKey = rpcReq.fileKey;

        // Currently the only tool that is not forwarded to the plugin is save_screenshots
        // If more are added we need to refactor to a better abstraction.
        if (rpcReq.tool === "save_screenshots") {
          const params = rpcReq.params ?? {};
          // Create a sender bound to the specific fileKey
          const sender = {
            sendWithParams: (
              requestType: string,
              nodeIds?: string[],
              sendParams?: Record<string, unknown>
            ) =>
              this.bridge.sendWithParams(
                requestType,
                nodeIds,
                sendParams,
                fileKey
              ),
          };
          const result = await executeSaveScreenshots(
            sender,
            params.items as Parameters<typeof executeSaveScreenshots>[1],
            params.format as ExportFormat | undefined,
            params.scale as number | undefined,
            params.clip as boolean | undefined
          );
          this.sendJSON(res, 200, { data: result });
          return;
        }

        const resp = await this.bridge.sendWithParams(
          rpcReq.tool,
          rpcReq.nodeIds,
          rpcReq.params,
          fileKey
        );

        this.sendJSON(
          res,
          200,
          resp.error ? { error: resp.error } : { data: resp.data }
        );
      } catch (err) {
        this.sendJSON(res, 200, {
          error: err instanceof Error ? err.message : String(err),
        });
      }
    });
  }

  private handleCreateIssue(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    let body = "";
    req.on("data", (chunk: Buffer) => {
      body += chunk.toString();
    });
    req.on("end", () => {
      try {
        const parsed = NewFlaggedIssueSchema.safeParse(JSON.parse(body));
        if (!parsed.success) {
          this.sendJSON(res, 400, { error: parsed.error.issues[0].message });
          return;
        }
        const created = this.issues.add(parsed.data);
        this.sendJSON(res, 200, { data: created });
      } catch (err) {
        this.sendJSON(res, 400, {
          error: err instanceof Error ? err.message : String(err),
        });
      }
    });
  }

  private handleListIssues(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    const url = new URL(req.url ?? "", "http://localhost");
    const fileKey = url.searchParams.get("fileKey") ?? undefined;
    this.sendJSON(res, 200, { data: this.issues.list(fileKey) });
  }

  private handleAckIssue(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    const url = new URL(req.url ?? "", "http://localhost");
    const id = url.pathname.slice(`${API}/issues/`.length);
    const ok = this.issues.ack(id);
    if (!ok) {
      this.sendJSON(res, 404, { error: `issue ${id} not found` });
      return;
    }
    this.sendJSON(res, 200, { data: { id } });
  }

  private handleGetCompareSession(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    this.sendJSON(res, 200, { data: this.compareSession.get() });
  }

  private handleSaveCompareSession(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    let body = "";
    req.on("data", (chunk: Buffer) => {
      body += chunk.toString();
    });
    req.on("end", () => {
      try {
        const parsed = CompareSessionSchema.safeParse(JSON.parse(body));
        if (!parsed.success) {
          this.sendJSON(res, 400, { error: parsed.error.issues[0].message });
          return;
        }
        const saved = this.compareSession.set(parsed.data);
        this.sendJSON(res, 200, { data: saved });
      } catch (err) {
        this.sendJSON(res, 400, {
          error: err instanceof Error ? err.message : String(err),
        });
      }
    });
  }

  private handleClearCompareSession(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    this.compareSession.clear();
    this.sendJSON(res, 200, { data: null });
  }

  private handleListCompareHistory(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    this.sendJSON(res, 200, { data: this.compareSession.listHistory() });
  }

  private handlePushCompareHistory(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    let body = "";
    req.on("data", (chunk: Buffer) => {
      body += chunk.toString();
    });
    req.on("end", () => {
      try {
        const parsed = CompareSessionSchema.safeParse(JSON.parse(body));
        if (!parsed.success) {
          this.sendJSON(res, 400, { error: parsed.error.issues[0].message });
          return;
        }
        const pushed = this.compareSession.pushHistory(parsed.data);
        this.sendJSON(res, 200, { data: pushed });
      } catch (err) {
        this.sendJSON(res, 400, {
          error: err instanceof Error ? err.message : String(err),
        });
      }
    });
  }

  private handlePinCompareHistory(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    const url = new URL(req.url ?? "", "http://localhost");
    const id = url.pathname.slice(`${API}/compare-session/history/`.length, -"/pin".length);
    let body = "";
    req.on("data", (chunk: Buffer) => {
      body += chunk.toString();
    });
    req.on("end", () => {
      try {
        const parsed = z.object({ pinned: z.boolean() }).safeParse(JSON.parse(body));
        if (!parsed.success) {
          this.sendJSON(res, 400, { error: parsed.error.issues[0].message });
          return;
        }
        const entry = this.compareSession.setPinned(id, parsed.data.pinned);
        if (!entry) {
          this.sendJSON(res, 404, { error: `history entry ${id} not found` });
          return;
        }
        this.sendJSON(res, 200, { data: entry });
      } catch (err) {
        this.sendJSON(res, 400, {
          error: err instanceof Error ? err.message : String(err),
        });
      }
    });
  }

  private handleDeleteCompareHistory(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    const url = new URL(req.url ?? "", "http://localhost");
    const id = url.pathname.slice(`${API}/compare-session/history/`.length);
    const ok = this.compareSession.deleteHistory(id);
    if (!ok) {
      this.sendJSON(res, 404, { error: `history entry ${id} not found` });
      return;
    }
    this.sendJSON(res, 200, { data: { id } });
  }

  private handleRequestReload(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    this.reloadSignal.request();
    this.sendJSON(res, 200, { data: { requested: true } });
  }

  private handleConsumeReload(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    this.sendJSON(res, 200, { data: { reload: this.reloadSignal.consume() } });
  }

  private sendJSON(
    res: http.ServerResponse,
    status: number,
    body: RPCResponse
  ): void {
    res.writeHead(status, { "Content-Type": "application/json" });
    res.end(JSON.stringify(body));
  }

  stop(): void {
    this.bridge.close();
    if (this.server) {
      this.server.close();
      this.server = null;
    }
  }
}

// Background service worker: captures the visible tab, crops to the
// requested region, and forwards a "flagged issue" to figma-map's bridge.
// Does no diffing or matching — that stays server-side (ADR-0001).
import { z } from "zod";
import { getOptions } from "./lib/options";
import { pingBridge, countPendingIssues, API_PREFIX } from "./lib/bridge";
import { dataUrlToBitmap, cropBitmapToBase64 } from "./lib/image";
import type { HitNode } from "./lib/hitmap";
import {
  RpcRequestSchema,
  RpcResponseSchema,
  CompareSessionDataSchema,
  CompareHistoryEntryDataSchema,
  FlaggedIssueDataSchema
} from "./protocol";
import type {
  AckIssueRequest,
  Bbox,
  CaptureRequest,
  CompareHistoryEntryData,
  CompareSessionData,
  DeleteCompareHistoryRequest,
  ExtensionRequest,
  ExtensionResponseMap,
  FetchScreenshotRequest,
  FlaggedIssueData,
  GetSelectionRequest,
  GetStatusRequest,
  GetSubtreeRequest,
  ListIssuesRequest,
  MatchZoomRequest,
  PinCompareHistoryRequest,
  PushCompareHistoryRequest,
  SendCompareRequest
} from "./protocol";

async function cropToBase64(dataUrl: string, bbox: Bbox, dpr: number): Promise<string> {
  const bitmap = await dataUrlToBitmap(dataUrl);
  const sx = Math.max(0, bbox.x * dpr);
  const sy = Math.max(0, bbox.y * dpr);
  const sw = Math.max(1, Math.round(bbox.width * dpr));
  const sh = Math.max(1, Math.round(bbox.height * dpr));
  return cropBitmapToBase64(bitmap, sx, sy, sw, sh);
}

interface IssuePayload {
  tabUrl: string;
  selector: string;
  bbox: Bbox;
  screenshotBase64: string;
  note?: string;
  figmaNodeId?: string;
  regionNodeId?: string;
  regionBounds?: Bbox;
  fileKey?: string;
}

// Shared by handleCapture and handleSendCompare — they differ in how
// `selector` and the figmaNodeId/fileKey precedence are derived, but the
// wire payload itself is one shape.
function buildIssuePayload(params: {
  tabUrl: string;
  selector: string;
  bbox: Bbox;
  screenshotBase64: string;
  note?: string;
  figmaNodeId?: string;
  regionNodeId?: string;
  regionBounds?: Bbox;
  fileKey?: string;
}): IssuePayload {
  return {
    tabUrl: params.tabUrl,
    selector: params.selector,
    bbox: params.bbox,
    screenshotBase64: params.screenshotBase64,
    note: params.note || undefined,
    figmaNodeId: params.figmaNodeId,
    regionNodeId: params.regionNodeId,
    regionBounds: params.regionBounds,
    fileKey: params.fileKey
  };
}

async function postIssue(issue: IssuePayload, bridgeUrl: string): Promise<void> {
  const resp = await fetch(bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/issues`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(issue)
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`bridge returned ${resp.status}: ${text}`);
  }
}

async function handleCapture(msg: CaptureRequest, sender: chrome.runtime.MessageSender): Promise<void> {
  const tab = sender.tab;
  if (!tab?.windowId) {
    throw new Error("no source tab for capture");
  }

  const dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId, { format: "png" });
  const screenshotBase64 = await cropToBase64(dataUrl, msg.bbox, msg.dpr || 1);
  const opts = await getOptions();

  await postIssue(
    buildIssuePayload({
      tabUrl: msg.tabUrl,
      selector: msg.selector,
      bbox: msg.bbox,
      screenshotBase64,
      note: msg.note,
      figmaNodeId: opts.figmaNodeId,
      fileKey: opts.fileKey
    }),
    opts.bridgeUrl
  );
}

// Captures the live page cropped to exactly the rect the user dragged the
// Figma overlay to — i.e. the region they've already hand-aligned — and
// flags it with the figmaNodeId that was fetched, so the agent can re-fetch
// the same Figma render and run `verify pixeldiff-images` for a precise
// number instead of eyeballing the overlay.
async function handleSendCompare(msg: SendCompareRequest, sender: chrome.runtime.MessageSender): Promise<void> {
  const tab = sender.tab;
  if (!tab?.windowId) {
    throw new Error("no source tab for capture");
  }

  const dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId, { format: "png" });
  const screenshotBase64 = await cropToBase64(dataUrl, msg.bbox, msg.dpr || 1);
  const opts = await getOptions();

  await postIssue(
    buildIssuePayload({
      tabUrl: msg.tabUrl,
      selector: msg.figmaNodeId ? `(overlay compare: figma node ${msg.figmaNodeId})` : "(overlay compare)",
      bbox: msg.bbox,
      screenshotBase64,
      note: msg.note,
      figmaNodeId: msg.figmaNodeId || opts.figmaNodeId,
      regionNodeId: msg.regionNodeId,
      regionBounds: msg.regionBounds,
      fileKey: msg.fileKey || opts.fileKey
    }),
    opts.bridgeUrl
  );
}

async function handleGetStatus(): Promise<{ connected: boolean; pending: number | null }> {
  const opts = await getOptions();
  const connected = await pingBridge(opts.bridgeUrl);
  const pending = connected ? await countPendingIssues(opts.bridgeUrl, opts.fileKey) : null;
  checkReload(); // piggyback on this poll, best-effort, doesn't block the status response
  return { connected, pending };
}

// Dev-only one-shot signal (backend/src/reloadSignal.ts): an agent/CLI
// POSTs /extension/reload, and the next time this checks, it consumes the
// flag and reloads itself — chrome.runtime.reload() needs no special
// permission (unlike chrome.management, which manages *other* extensions).
// Checked both on the status poll above (~30s, while any tab has the
// content script mounted) and on a chrome.alarms tick (so it still fires
// with no tabs open).
async function checkReload(): Promise<void> {
  try {
    const opts = await getOptions();
    const resp = await fetch(opts.bridgeUrl.replace(/\/$/, "") + "/extension/reload");
    if (!resp.ok) return;
    const body = await resp.json();
    if (body.data?.reload) {
      chrome.runtime.reload();
    }
  } catch {
    // bridge unreachable — nothing to do
  }
}

const RELOAD_CHECK_ALARM = "figma-map-reload-check";
chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === RELOAD_CHECK_ALARM) checkReload();
});
chrome.runtime.onInstalled.addListener(() => {
  chrome.alarms.create(RELOAD_CHECK_ALARM, { periodInMinutes: 1 });
});
chrome.runtime.onStartup.addListener(() => {
  chrome.alarms.create(RELOAD_CHECK_ALARM, { periodInMinutes: 1 });
});

interface ScreenshotExport {
  base64: string;
  width: number;
  height: number;
  cropX?: number;
  cropY?: number;
  cropW?: number;
  cropH?: number;
}

interface FetchedScreenshot {
  dataUrl: string;
  width: number;
  height: number;
}

// Renders a Figma node to a PNG via the bridge's existing get_screenshot RPC
// (the same call internal/figma/bridge.go makes) — no new Go code needed.
// Builds the request through RpcRequestSchema (throws on a malformed call
// site instead of silently sending bad JSON) and validates the response
// envelope through RpcResponseSchema — a backend that starts returning a
// differently-shaped body fails loudly here rather than typing through as
// `any` (ADR-0003 §3).
async function rpcCall(bridgeUrl: string, req: { tool: string; nodeIds?: string[]; params?: Record<string, unknown>; fileKey?: string }): Promise<unknown> {
  const parsedReq = RpcRequestSchema.parse(req);
  const resp = await fetch(bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/rpc`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(parsedReq)
  });
  const body = RpcResponseSchema.parse(await resp.json());
  if (!resp.ok || body.error) {
    throw new Error(body.error || `bridge returned ${resp.status}`);
  }
  return body.data;
}

async function handleFetchScreenshot(nodeId: string, fileKey?: string): Promise<FetchedScreenshot> {
  if (!nodeId.trim()) throw new Error("node id is required");
  const opts = await getOptions();

  const data = (await rpcCall(opts.bridgeUrl, {
    tool: "get_screenshot",
    nodeIds: [nodeId],
    params: { format: "PNG", scale: 1 },
    fileKey: fileKey || opts.fileKey
  })) as { exports?: ScreenshotExport[] } | undefined;
  const exports = data?.exports;
  if (!exports?.length) {
    throw new Error(`no screenshot returned for node ${nodeId}`);
  }

  const e = exports[0];
  let base64 = e.base64;
  let width = e.width;
  let height = e.height;
  if (e.cropX != null && e.cropY != null && e.cropW != null && e.cropH != null) {
    const bitmap = await dataUrlToBitmap(`data:image/png;base64,${base64}`);
    base64 = await cropBitmapToBase64(bitmap, e.cropX, e.cropY, e.cropW, e.cropH);
    width = e.cropW;
    height = e.cropH;
  }
  return { dataUrl: `data:image/png;base64,${base64}`, width, height };
}

// Asks the running Figma plugin what's currently selected, via the same
// /rpc relay handleFetchScreenshot uses — no new Go code needed.
async function handleGetSelection(fileKey?: string): Promise<HitNode[]> {
  const opts = await getOptions();
  const data = await rpcCall(opts.bridgeUrl, {
    tool: "get_selection",
    params: { depth: 0 },
    fileKey: fileKey || opts.fileKey
  });
  return (data as HitNode[] | undefined) ?? [];
}

// Fetches the selected node's full subtree (bounds + children) so the
// overlay can build a client-side hit-map. Depth-limited server-side —
// the Go Node type returns childCount instead of children past the cutoff,
// so no extra guard is needed here.
async function handleGetSubtree(nodeId: string, fileKey?: string): Promise<HitNode> {
  const opts = await getOptions();
  const data = (await rpcCall(opts.bridgeUrl, {
    tool: "get_node",
    nodeIds: [nodeId],
    // Hit-testing only ever reads id/name/bounds/children — lean skips
    // the styles/variables/dev-resources resolution get_node normally
    // does for every node, which is what made this hang on large
    // selections (a whole page, a big frame).
    params: { lean: true },
    fileKey: fileKey || opts.fileKey
  })) as HitNode | undefined;
  if (!data?.id) {
    throw new Error(`no node returned for ${nodeId}`);
  }
  return data;
}

// The overlay-compare session lives on the bridge (backend/src/compareSession.ts)
// — single source of truth, not chrome.storage.local — so these three are
// thin proxies to /compare-session, the same shape as the /issues proxies above.
async function handleGetCompareSession(): Promise<CompareSessionData | null> {
  const opts = await getOptions();
  const resp = await fetch(opts.bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/compare-session`);
  if (!resp.ok) {
    throw new Error(`bridge returned ${resp.status}`);
  }
  const body = await resp.json();
  return body.data ? CompareSessionDataSchema.parse(body.data) : null;
}

async function handleSaveCompareSession(session: CompareSessionData): Promise<void> {
  const opts = await getOptions();
  const resp = await fetch(opts.bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/compare-session`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(session)
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`bridge returned ${resp.status}: ${text}`);
  }
}

async function handleClearCompareSession(): Promise<void> {
  const opts = await getOptions();
  const resp = await fetch(opts.bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/compare-session`, {
    method: "DELETE"
  });
  if (!resp.ok) {
    throw new Error(`bridge returned ${resp.status}`);
  }
}

// Past compare sessions, pushed only when a new reference image loads (not
// on every drag/opacity tweak — those keep updating /compare-session
// above). Same store, backend/src/compareSession.ts's history side.
async function handleListCompareHistory(): Promise<CompareHistoryEntryData[]> {
  const opts = await getOptions();
  const resp = await fetch(opts.bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/compare-session/history`);
  if (!resp.ok) {
    throw new Error(`bridge returned ${resp.status}`);
  }
  const body = await resp.json();
  return z.array(CompareHistoryEntryDataSchema).parse(body.data ?? []);
}

async function handlePushCompareHistory(session: CompareSessionData): Promise<CompareHistoryEntryData> {
  const opts = await getOptions();
  const resp = await fetch(opts.bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/compare-session/history`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(session)
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`bridge returned ${resp.status}: ${text}`);
  }
  const body = await resp.json();
  return CompareHistoryEntryDataSchema.parse(body.data);
}

async function handlePinCompareHistory(id: string, pinned: boolean): Promise<CompareHistoryEntryData> {
  const opts = await getOptions();
  const resp = await fetch(opts.bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/compare-session/history/` + encodeURIComponent(id) + "/pin", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ pinned })
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`bridge returned ${resp.status}: ${text}`);
  }
  const body = await resp.json();
  return CompareHistoryEntryDataSchema.parse(body.data);
}

async function handleDeleteCompareHistory(id: string): Promise<void> {
  const opts = await getOptions();
  const resp = await fetch(opts.bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/compare-session/history/` + encodeURIComponent(id), {
    method: "DELETE"
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`bridge returned ${resp.status}: ${text}`);
  }
}

// Same inbox the CLI reads via `figma-map capture issues`/`capture ack`
// (internal/figma/issue.go) — the Issues window is just another consumer.
async function handleListIssues(fileKey?: string): Promise<FlaggedIssueData[]> {
  const opts = await getOptions();
  const url = opts.bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/issues` + (fileKey || opts.fileKey ? `?fileKey=${encodeURIComponent(fileKey || opts.fileKey || "")}` : "");
  const resp = await fetch(url);
  if (!resp.ok) {
    throw new Error(`bridge returned ${resp.status}`);
  }
  const body = await resp.json();
  return z.array(FlaggedIssueDataSchema).parse(body.data ?? []);
}

async function handleAckIssue(id: string): Promise<void> {
  const opts = await getOptions();
  const resp = await fetch(opts.bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/issues/` + encodeURIComponent(id), {
    method: "DELETE"
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`bridge returned ${resp.status}: ${text}`);
  }
}

// Computes the zoom factor that would make the tab's CSS-px viewport width
// equal targetWidth, and applies it. Lets someone working on a wide design
// (e.g. 1920px) on a narrower laptop screen (e.g. a 1440px MacBook Air) zoom
// out to exactly the width the design assumes, instead of guessing a zoom %.
async function handleMatchZoom(tabId: number, currentInnerWidth: number, targetWidth: number): Promise<number> {
  const currentZoom = await chrome.tabs.getZoom(tabId);
  const unzoomedWidth = currentInnerWidth * currentZoom;
  const newZoom = unzoomedWidth / targetWidth;
  await chrome.tabs.setZoom(tabId, newZoom);
  return newZoom;
}

// Dispatch table keyed by protocol.ts's message types — adding a message
// means adding one entry here and one to protocol.ts, not growing an
// if-chain. Each handler returns the exact response shape ExtensionRequest's
// `type` pins on the caller side (lib/*.ts), or void to fire-and-forget.
type Handler<T extends ExtensionRequest> = (
  msg: T,
  sender: chrome.runtime.MessageSender
) => Promise<ExtensionResponseMap[T["type"]]> | void;

const handlers: { [K in ExtensionRequest["type"]]: Handler<Extract<ExtensionRequest, { type: K }>> } = {
  FIGMA_MAP_CAPTURE: (msg, sender) =>
    handleCapture(msg, sender)
      .then(() => ({ ok: true }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_OPEN_OPTIONS: () => {
    chrome.runtime.openOptionsPage();
  },

  FIGMA_MAP_GET_STATUS: () => handleGetStatus(),

  FIGMA_MAP_FETCH_SCREENSHOT: (msg: FetchScreenshotRequest) =>
    handleFetchScreenshot(msg.nodeId, msg.fileKey)
      .then((result) => ({ ok: true, ...result }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_SEND_COMPARE: (msg, sender) =>
    handleSendCompare(msg, sender)
      .then(() => ({ ok: true }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_GET_SELECTION: (msg: GetSelectionRequest) =>
    handleGetSelection(msg.fileKey)
      .then((nodes) => ({ ok: true, nodes }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_GET_SUBTREE: (msg: GetSubtreeRequest) =>
    handleGetSubtree(msg.nodeId, msg.fileKey)
      .then((node) => ({ ok: true, node }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_MATCH_ZOOM: (msg: MatchZoomRequest, sender) => {
    const tabId = sender.tab?.id;
    if (!tabId) {
      return Promise.resolve({ ok: false, error: "no source tab" });
    }
    return handleMatchZoom(tabId, msg.currentInnerWidth, msg.targetWidth)
      .then((zoom) => ({ ok: true, zoom }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) }));
  },

  FIGMA_MAP_GET_COMPARE_SESSION: () =>
    handleGetCompareSession()
      .then((session) => ({ ok: true, session }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_SAVE_COMPARE_SESSION: (msg) =>
    handleSaveCompareSession(msg.session)
      .then(() => ({ ok: true }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_CLEAR_COMPARE_SESSION: () =>
    handleClearCompareSession()
      .then(() => ({ ok: true }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_LIST_ISSUES: (msg: ListIssuesRequest) =>
    handleListIssues(msg.fileKey)
      .then((issues) => ({ ok: true, issues }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_ACK_ISSUE: (msg: AckIssueRequest) =>
    handleAckIssue(msg.id)
      .then(() => ({ ok: true }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_LIST_COMPARE_HISTORY: () =>
    handleListCompareHistory()
      .then((entries) => ({ ok: true, entries }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_PUSH_COMPARE_HISTORY: (msg: PushCompareHistoryRequest) =>
    handlePushCompareHistory(msg.session)
      .then((entry) => ({ ok: true, entry }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_PIN_COMPARE_HISTORY: (msg: PinCompareHistoryRequest) =>
    handlePinCompareHistory(msg.id, msg.pinned)
      .then((entry) => ({ ok: true, entry }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) })),

  FIGMA_MAP_DELETE_COMPARE_HISTORY: (msg: DeleteCompareHistoryRequest) =>
    handleDeleteCompareHistory(msg.id)
      .then(() => ({ ok: true }))
      .catch((err) => ({ ok: false, error: err instanceof Error ? err.message : String(err) }))
};

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  const request = msg as ExtensionRequest;
  const handler = handlers[request.type] as Handler<ExtensionRequest> | undefined;
  if (!handler) return;

  const result = handler(request, sender);
  if (result instanceof Promise) {
    result.then(sendResponse);
    return true; // async response
  }
});

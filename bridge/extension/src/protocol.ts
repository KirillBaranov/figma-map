// Single source of truth for every chrome.runtime message exchanged between
// content scripts and the background service worker. One entry per message
// type, request AND response declared together, imported by both the
// background.ts dispatch table and the lib/*.ts typed callers — so the two
// sides cannot drift, the same reason internal/op.Registry generates the CLI
// and MCP surface from one declaration on the Go side.
import type { HitNode } from "./lib/hitmap";

export interface Bbox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface CaptureRequest {
  type: "FIGMA_MAP_CAPTURE";
  bbox: Bbox;
  dpr: number;
  selector: string;
  tabUrl: string;
  note: string;
}
export interface CaptureResponse {
  ok: boolean;
  error?: string;
}

export interface OpenOptionsRequest {
  type: "FIGMA_MAP_OPEN_OPTIONS";
}

export interface GetStatusRequest {
  type: "FIGMA_MAP_GET_STATUS";
}
export interface GetStatusResponse {
  connected: boolean;
  pending: number | null;
}

export interface FetchScreenshotRequest {
  type: "FIGMA_MAP_FETCH_SCREENSHOT";
  nodeId: string;
  fileKey?: string;
}
export interface FetchScreenshotResponse {
  ok: boolean;
  error?: string;
  dataUrl?: string;
  width?: number;
  height?: number;
}

export interface SendCompareRequest {
  type: "FIGMA_MAP_SEND_COMPARE";
  bbox: Bbox;
  dpr: number;
  tabUrl: string;
  note?: string;
  figmaNodeId?: string;
  regionNodeId?: string;
  regionBounds?: Bbox;
  fileKey?: string;
}
export interface SendCompareResponse {
  ok: boolean;
  error?: string;
}

export interface GetSelectionRequest {
  type: "FIGMA_MAP_GET_SELECTION";
  fileKey?: string;
}
export interface GetSelectionResponse {
  ok: boolean;
  error?: string;
  nodes?: HitNode[];
}

export interface GetSubtreeRequest {
  type: "FIGMA_MAP_GET_SUBTREE";
  nodeId: string;
  fileKey?: string;
}
export interface GetSubtreeResponse {
  ok: boolean;
  error?: string;
  node?: HitNode;
}

export interface MatchZoomRequest {
  type: "FIGMA_MAP_MATCH_ZOOM";
  currentInnerWidth: number;
  targetWidth: number;
}
export interface MatchZoomResponse {
  ok: boolean;
  error?: string;
  zoom?: number;
}

// The overlay-compare session — single source of truth on the bridge
// (bridge/server/src/compareSession.ts), not chrome.storage.local. Mirrors
// bridge's CompareSession type field-for-field.
export interface CompareSessionData {
  image: string;
  naturalW: number;
  naturalH: number;
  figmaW?: number;
  figmaH?: number;
  fetchedNodeId?: string;
  nodeId: string;
  pos: { x: number; y: number };
  scale: number;
  opacity: number;
  diffMode: boolean;
  syncScroll: boolean;
  panelPos?: { x: number; y: number };
}

export interface GetCompareSessionRequest {
  type: "FIGMA_MAP_GET_COMPARE_SESSION";
}
export interface GetCompareSessionResponse {
  ok: boolean;
  error?: string;
  session?: CompareSessionData | null;
}

export interface SaveCompareSessionRequest {
  type: "FIGMA_MAP_SAVE_COMPARE_SESSION";
  session: CompareSessionData;
}
export interface SaveCompareSessionResponse {
  ok: boolean;
  error?: string;
}

export interface ClearCompareSessionRequest {
  type: "FIGMA_MAP_CLEAR_COMPARE_SESSION";
}
export interface ClearCompareSessionResponse {
  ok: boolean;
  error?: string;
}

// Mirrors bridge/server/src/types.ts's FlaggedIssue field-for-field.
export interface FlaggedIssueData {
  id: string;
  tabUrl: string;
  selector: string;
  figmaNodeId?: string;
  regionNodeId?: string;
  regionBounds?: Bbox;
  fileKey?: string;
  bbox: Bbox;
  screenshotBase64: string;
  diffPct?: number;
  note?: string;
  createdAt: string;
}

export interface ListIssuesRequest {
  type: "FIGMA_MAP_LIST_ISSUES";
  fileKey?: string;
}
export interface ListIssuesResponse {
  ok: boolean;
  error?: string;
  issues?: FlaggedIssueData[];
}

export interface AckIssueRequest {
  type: "FIGMA_MAP_ACK_ISSUE";
  id: string;
}
export interface AckIssueResponse {
  ok: boolean;
  error?: string;
}

// A past compare session kept for re-activation. Mirrors
// bridge/server/src/types.ts's CompareHistoryEntry field-for-field.
export interface CompareHistoryEntryData extends CompareSessionData {
  id: string;
  createdAt: string;
  pinned: boolean;
}

export interface ListCompareHistoryRequest {
  type: "FIGMA_MAP_LIST_COMPARE_HISTORY";
}
export interface ListCompareHistoryResponse {
  ok: boolean;
  error?: string;
  entries?: CompareHistoryEntryData[];
}

export interface PushCompareHistoryRequest {
  type: "FIGMA_MAP_PUSH_COMPARE_HISTORY";
  session: CompareSessionData;
}
export interface PushCompareHistoryResponse {
  ok: boolean;
  error?: string;
  entry?: CompareHistoryEntryData;
}

export interface PinCompareHistoryRequest {
  type: "FIGMA_MAP_PIN_COMPARE_HISTORY";
  id: string;
  pinned: boolean;
}
export interface PinCompareHistoryResponse {
  ok: boolean;
  error?: string;
  entry?: CompareHistoryEntryData;
}

export interface DeleteCompareHistoryRequest {
  type: "FIGMA_MAP_DELETE_COMPARE_HISTORY";
  id: string;
}
export interface DeleteCompareHistoryResponse {
  ok: boolean;
  error?: string;
}

export type ExtensionRequest =
  | CaptureRequest
  | OpenOptionsRequest
  | GetStatusRequest
  | FetchScreenshotRequest
  | SendCompareRequest
  | GetSelectionRequest
  | GetSubtreeRequest
  | MatchZoomRequest
  | GetCompareSessionRequest
  | SaveCompareSessionRequest
  | ClearCompareSessionRequest
  | ListIssuesRequest
  | AckIssueRequest
  | ListCompareHistoryRequest
  | PushCompareHistoryRequest
  | PinCompareHistoryRequest
  | DeleteCompareHistoryRequest;

export interface ExtensionResponseMap {
  FIGMA_MAP_CAPTURE: CaptureResponse;
  FIGMA_MAP_OPEN_OPTIONS: void;
  FIGMA_MAP_GET_STATUS: GetStatusResponse;
  FIGMA_MAP_FETCH_SCREENSHOT: FetchScreenshotResponse;
  FIGMA_MAP_SEND_COMPARE: SendCompareResponse;
  FIGMA_MAP_GET_SELECTION: GetSelectionResponse;
  FIGMA_MAP_GET_SUBTREE: GetSubtreeResponse;
  FIGMA_MAP_MATCH_ZOOM: MatchZoomResponse;
  FIGMA_MAP_GET_COMPARE_SESSION: GetCompareSessionResponse;
  FIGMA_MAP_SAVE_COMPARE_SESSION: SaveCompareSessionResponse;
  FIGMA_MAP_CLEAR_COMPARE_SESSION: ClearCompareSessionResponse;
  FIGMA_MAP_LIST_ISSUES: ListIssuesResponse;
  FIGMA_MAP_ACK_ISSUE: AckIssueResponse;
  FIGMA_MAP_LIST_COMPARE_HISTORY: ListCompareHistoryResponse;
  FIGMA_MAP_PUSH_COMPARE_HISTORY: PushCompareHistoryResponse;
  FIGMA_MAP_PIN_COMPARE_HISTORY: PinCompareHistoryResponse;
  FIGMA_MAP_DELETE_COMPARE_HISTORY: DeleteCompareHistoryResponse;
}

// Typed wrapper around chrome.runtime.sendMessage — the request's `type`
// pins the response shape, so a caller and background.ts's handler for that
// same `type` cannot silently disagree on either end's shape.
export function sendExtensionMessage<T extends ExtensionRequest>(
  message: T
): Promise<ExtensionResponseMap[T["type"]]> {
  return chrome.runtime.sendMessage(message);
}

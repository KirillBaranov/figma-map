// Single source of truth for every chrome.runtime message exchanged between
// content scripts and the background service worker. One entry per message
// type, request AND response declared together, imported by both the
// background.ts dispatch table and the lib/*.ts typed callers — so the two
// sides cannot drift, the same reason internal/op.Registry generates the CLI
// and MCP surface from one declaration on the Go side.
import { z } from "zod";
import type { HitNode } from "./lib/hitmap";

// --- backend HTTP wire contract (ADR-0003 §3) ---
// These mirror backend/src/types.ts's zod schemas field-for-field. Not a
// cross-package import: backend (Node/tsc) and this extension (browser/vite)
// are separate npm packages with no workspace wired up between them, so
// backend/src/types.ts stays the canonical source and this is a second,
// independently-validated copy — a malformed backend response still fails
// loudly here via .parse(), it just isn't the literal same schema object.
export const RpcRequestSchema = z.object({
  tool: z.string(),
  nodeIds: z.array(z.string()).optional(),
  params: z.record(z.string(), z.unknown()).optional(),
  fileKey: z.string().optional()
});
export type RpcRequest = z.infer<typeof RpcRequestSchema>;

export const RpcResponseSchema = z.object({
  data: z.unknown().optional(),
  error: z.string().optional()
});
export type RpcResponse = z.infer<typeof RpcResponseSchema>;

export const CompareSessionPosSchema = z.object({
  x: z.number(),
  y: z.number()
});

export const CompareSessionDataSchema = z.object({
  image: z.string(),
  naturalW: z.number(),
  naturalH: z.number(),
  figmaW: z.number().optional(),
  figmaH: z.number().optional(),
  fetchedNodeId: z.string().optional(),
  nodeId: z.string(),
  pos: CompareSessionPosSchema,
  scale: z.number(),
  opacity: z.number(),
  diffMode: z.boolean(),
  syncScroll: z.boolean(),
  panelPos: CompareSessionPosSchema.optional(),
  updatedAt: z.string().optional()
});
export type CompareSessionData = z.infer<typeof CompareSessionDataSchema>;

export const CompareHistoryEntryDataSchema = CompareSessionDataSchema.extend({
  id: z.string(),
  createdAt: z.string(),
  pinned: z.boolean()
});
export type CompareHistoryEntryData = z.infer<typeof CompareHistoryEntryDataSchema>;

const BboxSchema = z.object({
  x: z.number(),
  y: z.number(),
  width: z.number(),
  height: z.number()
});

export const FlaggedIssueDataSchema = z.object({
  id: z.string(),
  tabUrl: z.string(),
  selector: z.string(),
  figmaNodeId: z.string().optional(),
  regionNodeId: z.string().optional(),
  regionBounds: BboxSchema.optional(),
  fileKey: z.string().optional(),
  bbox: BboxSchema,
  screenshotBase64: z.string(),
  diffPct: z.number().optional(),
  note: z.string().optional(),
  createdAt: z.string()
});
export type FlaggedIssueData = z.infer<typeof FlaggedIssueDataSchema>;

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

// The overlay-compare session — single source of truth on the backend
// (backend/src/compareSession.ts), not chrome.storage.local. CompareSessionData
// itself is defined above as a zod schema (ADR-0003 §3).

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

// FlaggedIssueData itself is defined above as a zod schema (ADR-0003 §3).

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

// CompareHistoryEntryData itself is defined above as a zod schema (ADR-0003 §3).

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

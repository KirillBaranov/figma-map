import { z } from "zod";

// BridgeRequest/BridgeResponse are the internal WS framing between the
// backend and the Figma plugin — never seen by the extension or the Go
// client, so they stay plain interfaces (not part of the shared contract
// zod covers below).
export interface BridgeRequest {
  type: string;
  requestId: string;
  nodeIds?: string[];
  params?: Record<string, unknown>;
}

export interface BridgeResponse {
  type: string;
  requestId: string;
  data?: unknown;
  error?: string;
}

export interface ConnectedFile {
  fileKey: string;
  fileName: string;
}

export enum Role {
  Unknown = 0,
  Leader = 1,
  Follower = 2,
}

// --- Shared wire contract (ADR-0003 §2/§3: canonical source for the
// backend's HTTP API. bridge/extension/src/protocol.ts mirrors these shapes
// as its own zod schemas — see that file for why it isn't a direct
// cross-package import.) ---

export const RPCRequestSchema = z.object({
  tool: z.string(),
  nodeIds: z.array(z.string()).optional(),
  params: z.record(z.string(), z.unknown()).optional(),
  fileKey: z.string().optional(),
});
export type RPCRequest = z.infer<typeof RPCRequestSchema>;

export const RPCResponseSchema = z.object({
  data: z.unknown().optional(),
  error: z.string().optional(),
});
export type RPCResponse = z.infer<typeof RPCResponseSchema>;

export const IssueBBoxSchema = z.object({
  x: z.number(),
  y: z.number(),
  width: z.number(),
  height: z.number(),
});
export type IssueBBox = z.infer<typeof IssueBBoxSchema>;

export const DiffRegionSchema = IssueBBoxSchema.extend({
  diffPct: z.number(),
});
export type DiffRegion = z.infer<typeof DiffRegionSchema>;

export const FlaggedIssueSchema = z.object({
  id: z.string(),
  tabUrl: z.string(),
  selector: z.string(),
  figmaNodeId: z.string().optional(),
  // Descendant of figmaNodeId pinpointed via the overlay's click-to-node hit-map.
  regionNodeId: z.string().optional(),
  regionBounds: IssueBBoxSchema.optional(),
  fileKey: z.string().optional(),
  bbox: IssueBBoxSchema,
  screenshotBase64: z.string(),
  diffPct: z.number().optional(),
  diffRegions: z.array(DiffRegionSchema).optional(),
  note: z.string().optional(),
  createdAt: z.string(),
});
export type FlaggedIssue = z.infer<typeof FlaggedIssueSchema>;

export const NewFlaggedIssueSchema = FlaggedIssueSchema.omit({ id: true, createdAt: true });
export type NewFlaggedIssue = z.infer<typeof NewFlaggedIssueSchema>;

export const CompareSessionPosSchema = z.object({
  x: z.number(),
  y: z.number(),
});
export type CompareSessionPos = z.infer<typeof CompareSessionPosSchema>;

// The overlay-compare session (which reference image is loaded, its Figma
// node id, and how it's positioned against the live page) — single source
// of truth on the backend instead of the extension's own chrome.storage.local,
// same reasoning as FlaggedIssue: this is domain data, not per-tab UI state.
export const CompareSessionSchema = z.object({
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
  // Stamped server-side on every set() — drives the active slot's 7-day TTL (ADR-0003 §4).
  updatedAt: z.string().optional(),
});
export type CompareSession = z.infer<typeof CompareSessionSchema>;

// A past compare session kept for re-activation — pushed only when a new
// reference image is loaded (not on every drag/opacity tweak to the active
// one). Pinned entries are exempt from TTL and the auto-eviction cap.
export const CompareHistoryEntrySchema = CompareSessionSchema.extend({
  id: z.string(),
  createdAt: z.string(),
  pinned: z.boolean(),
});
export type CompareHistoryEntry = z.infer<typeof CompareHistoryEntrySchema>;

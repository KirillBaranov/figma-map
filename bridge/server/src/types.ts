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

export interface RPCRequest {
  tool: string;
  nodeIds?: string[];
  params?: Record<string, unknown>;
  fileKey?: string;
}

export interface RPCResponse {
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

export interface IssueBBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface DiffRegion {
  x: number;
  y: number;
  width: number;
  height: number;
  diffPct: number;
}

export interface FlaggedIssue {
  id: string;
  tabUrl: string;
  selector: string;
  figmaNodeId?: string;
  /** Descendant of figmaNodeId pinpointed via the overlay's click-to-node hit-map. */
  regionNodeId?: string;
  regionBounds?: IssueBBox;
  fileKey?: string;
  bbox: IssueBBox;
  screenshotBase64: string;
  diffPct?: number;
  diffRegions?: DiffRegion[];
  note?: string;
  createdAt: string;
}

export interface CompareSessionPos {
  x: number;
  y: number;
}

// The overlay-compare session (which reference image is loaded, its Figma
// node id, and how it's positioned against the live page) — single source
// of truth on the bridge instead of the extension's own chrome.storage.local,
// same reasoning as FlaggedIssue: this is domain data, not per-tab UI state.
export interface CompareSession {
  image: string;
  naturalW: number;
  naturalH: number;
  figmaW?: number;
  figmaH?: number;
  fetchedNodeId?: string;
  nodeId: string;
  pos: CompareSessionPos;
  scale: number;
  opacity: number;
  diffMode: boolean;
  syncScroll: boolean;
  panelPos?: CompareSessionPos;
}

// A past compare session kept for re-activation — pushed only when a new
// reference image is loaded (not on every drag/opacity tweak to the active
// one). Pinned entries are exempt from the auto-eviction cap.
export interface CompareHistoryEntry extends CompareSession {
  id: string;
  createdAt: string;
  pinned: boolean;
}

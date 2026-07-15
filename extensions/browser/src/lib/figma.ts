import { sendExtensionMessage, type Bbox } from "../protocol";
import type { HitNode } from "./hitmap";

export interface FigmaScreenshot {
  dataUrl: string;
  width: number;
  height: number;
}

// Always goes through the background service worker — see lib/status.ts for
// why content-script-initiated fetches to the bridge are unreliable.
export async function fetchFigmaScreenshot(nodeId: string, fileKey?: string): Promise<FigmaScreenshot> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_FETCH_SCREENSHOT", nodeId, fileKey });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to fetch screenshot from Figma");
  }
  return { dataUrl: resp.dataUrl!, width: resp.width!, height: resp.height! };
}

export interface SendCompareInput {
  bbox: Bbox;
  dpr: number;
  tabUrl: string;
  note?: string;
  figmaNodeId?: string;
  fileKey?: string;
  // The specific descendant of figmaNodeId resolved by the overlay's
  // click-to-node hit-map, when selection mode pinpointed a sub-element
  // rather than just the root node that was screenshotted.
  regionNodeId?: string;
  regionBounds?: Bbox;
}

// Flags the hand-aligned overlay region as an issue for the agent — see
// lib/status.ts for why this goes through the background worker rather than
// fetching directly from the content script.
export async function sendCompareToAgent(input: SendCompareInput): Promise<void> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_SEND_COMPARE", ...input });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to send to agent");
  }
}

// Screenshots exactly the bbox the overlay currently occupies, for
// useDiffSnapshot's amplified diff — no /issues POST, just the pixels.
export async function captureViewport(bbox: Bbox, dpr: number): Promise<string> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_CAPTURE_VIEWPORT", bbox, dpr });
  if (!resp?.ok || !resp.dataUrl) {
    throw new Error(resp?.error || "failed to capture viewport");
  }
  return resp.dataUrl;
}

// Asks the running Figma plugin what's currently selected, for "Use Figma
// selection" mode in the overlay.
export async function getFigmaSelection(fileKey?: string): Promise<HitNode[]> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_GET_SELECTION", fileKey });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to read Figma selection");
  }
  return resp.nodes ?? [];
}

// Fetches a node's full subtree (bounds + children) to build the overlay's
// client-side hit-map.
export async function getFigmaSubtree(nodeId: string, fileKey?: string): Promise<HitNode> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_GET_SUBTREE", nodeId, fileKey });
  if (!resp?.ok || !resp.node) {
    throw new Error(resp?.error || "failed to fetch node subtree from Figma");
  }
  return resp.node;
}

// Sets the tab's browser zoom so its CSS-px viewport width equals
// targetWidth — e.g. dialing a 1440px laptop screen to exactly the 1920px a
// design assumes, instead of guessing a zoom percentage by eye.
export async function matchZoomToWidth(targetWidth: number): Promise<number> {
  const resp = await sendExtensionMessage({
    type: "FIGMA_MAP_MATCH_ZOOM",
    currentInnerWidth: window.innerWidth,
    targetWidth
  });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to set zoom");
  }
  return resp.zoom as number;
}

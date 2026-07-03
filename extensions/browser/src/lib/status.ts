import { sendExtensionMessage, type GetStatusResponse } from "../protocol";

export type BridgeStatus = GetStatusResponse;

// Always goes through the background service worker — a fetch issued
// directly from a content script runs in the host page's context and is
// subject to that page's CSP/CORS, not this extension's host_permissions.
export async function getStatus(): Promise<BridgeStatus> {
  return sendExtensionMessage({ type: "FIGMA_MAP_GET_STATUS" });
}

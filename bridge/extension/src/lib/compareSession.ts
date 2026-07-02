import { sendExtensionMessage, type CompareHistoryEntryData, type CompareSessionData } from "../protocol";

export type CompareState = CompareSessionData;
export type { CompareHistoryEntryData };

// Single source of truth lives on the bridge (bridge/server/src/compareSession.ts),
// not chrome.storage.local — the extension only ever reads/writes it through
// the background worker, same reasoning as lib/status.ts's getStatus().
export async function saveCompareState(state: CompareState): Promise<void> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_SAVE_COMPARE_SESSION", session: state });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to save compare session");
  }
}

export async function loadCompareState(): Promise<CompareState | null> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_GET_COMPARE_SESSION" });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to load compare session");
  }
  return resp.session ?? null;
}

export async function clearCompareState(): Promise<void> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_CLEAR_COMPARE_SESSION" });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to clear compare session");
  }
}

// Past sessions, pushed only when a new reference image loads — see
// content/hooks/useFigmaCompare.ts for the call sites.
export async function listCompareHistory(): Promise<CompareHistoryEntryData[]> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_LIST_COMPARE_HISTORY" });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to list compare history");
  }
  return resp.entries ?? [];
}

export async function pushCompareHistory(session: CompareState): Promise<CompareHistoryEntryData> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_PUSH_COMPARE_HISTORY", session });
  if (!resp?.ok || !resp.entry) {
    throw new Error(resp?.error || "failed to push compare history");
  }
  return resp.entry;
}

export async function pinCompareHistory(id: string, pinned: boolean): Promise<CompareHistoryEntryData> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_PIN_COMPARE_HISTORY", id, pinned });
  if (!resp?.ok || !resp.entry) {
    throw new Error(resp?.error || "failed to pin compare history entry");
  }
  return resp.entry;
}

export async function deleteCompareHistory(id: string): Promise<void> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_DELETE_COMPARE_HISTORY", id });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to delete compare history entry");
  }
}

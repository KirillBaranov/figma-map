// Every versioned data-plane endpoint the backend exposes lives under this
// prefix (ADR-0003 §2). /ping and /extension/reload are deliberately left
// outside it — infrastructure, not the data API.
export const API_PREFIX = "/api/v1";

export async function pingBridge(bridgeUrl: string): Promise<boolean> {
  try {
    const resp = await fetch(bridgeUrl.replace(/\/$/, "") + "/ping", { method: "GET" });
    return resp.ok;
  } catch {
    return false;
  }
}

export async function countPendingIssues(bridgeUrl: string, fileKey?: string): Promise<number | null> {
  try {
    const url = bridgeUrl.replace(/\/$/, "") + `${API_PREFIX}/issues` + (fileKey ? `?fileKey=${encodeURIComponent(fileKey)}` : "");
    const resp = await fetch(url);
    if (!resp.ok) return null;
    const body = await resp.json();
    return Array.isArray(body.data) ? body.data.length : null;
  } catch {
    return null;
  }
}

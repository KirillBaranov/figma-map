import { sendExtensionMessage } from "../protocol";
import type { FlaggedIssueData } from "../protocol";

export type { FlaggedIssueData };

// Same bridge inbox the CLI reads via `figma-map capture issues`/`capture
// ack` — the Issues window is just another consumer of the same data.
export async function listIssues(fileKey?: string): Promise<FlaggedIssueData[]> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_LIST_ISSUES", fileKey });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to list issues");
  }
  return resp.issues ?? [];
}

export async function ackIssue(id: string): Promise<void> {
  const resp = await sendExtensionMessage({ type: "FIGMA_MAP_ACK_ISSUE", id });
  if (!resp?.ok) {
    throw new Error(resp?.error || "failed to ack issue");
  }
}

import { useEffect, useState } from "react";
import { Button, CheckIcon, RefreshIcon, Tooltip } from "../kit";
import { ackIssue, listIssues, type FlaggedIssueData } from "../lib/issues";
import type { Pos } from "./hooks/useDraggable";
import { Window } from "./Window";

interface IssuesWindowProps {
  fileKey?: string;
  onClose: () => void;
}

function timeAgo(iso: string): string {
  const minutes = Math.round((Date.now() - new Date(iso).getTime()) / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

// Same bridge inbox the CLI reads via `figma-map capture issues`/`capture
// ack` (internal/figma/issue.go) — browsing/acking here doesn't duplicate
// the bar's pending-count poll, it fetches on open plus manual refresh.
export function IssuesWindow({ fileKey, onClose }: IssuesWindowProps) {
  // Not persisted — unlike the compare window's panelPos (bridge-backed),
  // this is a transient view that always reopens at Window's default anchor.
  const [pos, setPos] = useState<Pos | null>(null);
  const [issues, setIssues] = useState<FlaggedIssueData[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [acking, setAcking] = useState<string | null>(null);

  async function refresh() {
    setLoading(true);
    setError(null);
    try {
      setIssues(await listIssues(fileKey));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    refresh();
    // Only on open — no polling loop, see the module comment above.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function onAck(id: string) {
    setAcking(id);
    try {
      await ackIssue(id);
      setIssues((prev) => prev.filter((issue) => issue.id !== id));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setAcking(null);
    }
  }

  return (
    <Window title="Issues" pos={pos} onPosChange={setPos} onClose={onClose}>
      <div className="fm-issues-header">
        <span className="fm-compare-hint">{issues.length} open</span>
        <Tooltip label="Refresh" side="bottom">
          <Button variant="ghost" onClick={refresh} disabled={loading} aria-label="Refresh">
            <RefreshIcon />
          </Button>
        </Tooltip>
      </div>

      {issues.length === 0 && !loading && <div className="fm-compare-hint">No open issues.</div>}

      {issues.map((issue) => (
        <div className="fm-issue-row" key={issue.id}>
          <img className="fm-issue-thumb" src={`data:image/png;base64,${issue.screenshotBase64}`} alt="" />
          <div className="fm-issue-body">
            <div className="fm-issue-selector">{issue.selector}</div>
            {issue.figmaNodeId && <div className="fm-compare-hint">figma: {issue.figmaNodeId}</div>}
            {issue.note && <div className="fm-issue-note">{issue.note}</div>}
            <div className="fm-compare-hint">{timeAgo(issue.createdAt)}</div>
          </div>
          <Tooltip label="Mark handled" side="bottom">
            <Button variant="secondary" onClick={() => onAck(issue.id)} disabled={acking === issue.id} aria-label="Ack">
              <CheckIcon />
            </Button>
          </Tooltip>
        </div>
      ))}

      {error && <div className="fm-compare-error">{error}</div>}
    </Window>
  );
}

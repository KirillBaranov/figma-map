import { CompareIcon, CrosshairIcon, InboxIcon, SettingsIcon, Tooltip } from "../kit";

type Status = "pending" | "connected" | "disconnected";
type ActiveWindow = "compare" | "issues" | null;

interface BarProps {
  status: Status;
  pending: number | null;
  selecting: boolean;
  activeWindow: ActiveWindow;
  onToggleSelect: () => void;
  onToggleCompare: () => void;
  onToggleIssues: () => void;
  onOpenSettings: () => void;
}

const STATUS_LABEL: Record<Status, string> = {
  connected: "connected",
  disconnected: "disconnected",
  pending: "checking…"
};

// Fixed, centered, never draggable (unlike the windows it opens above
// itself) — always fully visible, no expand/collapse state.
export function Bar({ status, pending, selecting, activeWindow, onToggleSelect, onToggleCompare, onToggleIssues, onOpenSettings }: BarProps) {
  return (
    <div className="fm-reset fm-bar-root">
      <div className="fm-bar-status">
        <span className={`fm-dot fm-bar-dot-${status}`} />
        <span>{STATUS_LABEL[status]}</span>
      </div>
      <div className="fm-bar-divider" />
      <div className="fm-bar-actions">
        <Tooltip label={selecting ? "Selecting… (Esc to cancel)" : "Select region"}>
          <button
            type="button"
            className={`fm-bar-btn ${selecting ? "fm-bar-btn-active" : ""}`}
            onClick={onToggleSelect}
            disabled={status === "disconnected"}
            aria-label="Select region"
          >
            <CrosshairIcon />
          </button>
        </Tooltip>
        <Tooltip label="Overlay compare">
          <button
            type="button"
            className={`fm-bar-btn ${activeWindow === "compare" ? "fm-bar-btn-active" : ""}`}
            onClick={onToggleCompare}
            aria-label="Overlay compare"
          >
            <CompareIcon />
          </button>
        </Tooltip>
        <Tooltip label={pending !== null && pending > 0 ? `Issues (${pending} pending)` : "Issues"}>
          <button
            type="button"
            className={`fm-bar-btn ${activeWindow === "issues" ? "fm-bar-btn-active" : ""}`}
            onClick={onToggleIssues}
            aria-label="Issues"
          >
            <InboxIcon />
            {pending !== null && pending > 0 && <span className="fm-bar-badge">{pending}</span>}
          </button>
        </Tooltip>
        <Tooltip label="Settings">
          <button type="button" className="fm-bar-btn" onClick={onOpenSettings} aria-label="Settings">
            <SettingsIcon />
          </button>
        </Tooltip>
      </div>
    </div>
  );
}

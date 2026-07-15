import { useRef } from "react";
import { CompareIcon, CrosshairIcon, InboxIcon, SettingsIcon, Tooltip } from "../kit";
import { useDraggable, type Pos } from "./hooks/useDraggable";

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
  // null = default bottom-center CSS position.
  pos: Pos | null;
  onPosChange: (pos: Pos | null) => void;
}

const STATUS_LABEL: Record<Status, string> = {
  connected: "connected",
  disconnected: "disconnected",
  pending: "checking…"
};

const FALLBACK_POS: Pos = { x: 20, y: 20 };

// Always fully visible, no expand/collapse state — unlike the windows it
// opens above itself, this is the only way back to them, so hiding it
// would strand the user. Draggable instead: the status readout (not the
// buttons) is the grab handle, so it can be moved out of the way of
// whatever it's covering. Double-click the handle to reset to the default
// bottom-center position.
export function Bar({
  status,
  pending,
  selecting,
  activeWindow,
  onToggleSelect,
  onToggleCompare,
  onToggleIssues,
  onOpenSettings,
  pos,
  onPosChange
}: BarProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const drag = useDraggable(onPosChange, FALLBACK_POS);

  function onHandleMouseDown(e: React.MouseEvent) {
    const rect = rootRef.current?.getBoundingClientRect();
    const current = pos ?? (rect ? { x: rect.left, y: rect.top } : FALLBACK_POS);
    drag.startDrag(e, current);
  }

  return (
    <div
      ref={rootRef}
      className={`fm-reset fm-bar-root ${drag.dragging ? "fm-bar-dragging" : ""}`}
      style={pos ? { left: pos.x, top: pos.y, bottom: "auto", transform: "none" } : undefined}
    >
      <div
        className="fm-bar-status fm-bar-handle"
        onMouseDown={onHandleMouseDown}
        onDoubleClick={() => onPosChange(null)}
        title="Drag to move · double-click to reset position"
      >
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

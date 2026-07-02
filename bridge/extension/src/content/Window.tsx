import { useEffect, useRef, type ReactNode } from "react";
import { CloseIcon, Tooltip } from "../kit";
import { useDraggable, type Pos } from "./hooks/useDraggable";

interface WindowProps {
  title: string;
  pos: Pos | null;
  onPosChange: (pos: Pos) => void;
  onClose: () => void;
  onDrop?: (e: React.DragEvent) => void;
  onDragOver?: (e: React.DragEvent) => void;
  onDragLeave?: () => void;
  onPaste?: (e: React.ClipboardEvent) => void;
  draggingOver?: boolean;
  children: ReactNode;
}

const FALLBACK_POS: Pos = { x: 20, y: 20 };

// Generic draggable window frame — title bar (drag handle + close) around a
// content slot. The bar (Bar.tsx) is fixed and never draggable; every
// window that opens above it (Overlay compare, Issues) shares this frame
// instead of building its own title bar/drag wiring.
export function Window({
  title,
  pos,
  onPosChange,
  onClose,
  onDrop,
  onDragOver,
  onDragLeave,
  onPaste,
  draggingOver,
  children
}: WindowProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const drag = useDraggable(onPosChange, FALLBACK_POS);

  // Same reasoning as the old CompareOverlay: nothing grabs focus just by
  // rendering this panel, so without it Cmd+V does nothing because the host
  // page (not us) has focus.
  useEffect(() => {
    rootRef.current?.focus();
  }, []);

  function onTitleMouseDown(e: React.MouseEvent) {
    const current: Pos = pos ?? rootRef.current?.getBoundingClientRect() ?? FALLBACK_POS;
    drag.startDrag(e, current);
  }

  return (
    <div
      ref={rootRef}
      tabIndex={-1}
      className={`fm-reset fm-panel fm-window ${draggingOver ? "fm-window-dropping" : ""} ${drag.dragging ? "fm-window-dragging" : ""}`}
      style={pos ? { left: pos.x, top: pos.y, bottom: "auto", transform: "none" } : undefined}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
      onPaste={onPaste}
    >
      <div className="fm-window-title" onMouseDown={onTitleMouseDown}>
        <span>{title}</span>
        <Tooltip label="Close" side="bottom">
          <button type="button" className="fm-window-close" onClick={onClose} aria-label="Close">
            <CloseIcon />
          </button>
        </Tooltip>
      </div>
      <div className="fm-window-body">{children}</div>
    </div>
  );
}

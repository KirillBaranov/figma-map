import { useRef, useState, type ReactNode } from "react";
import { computePosition, flip, offset, shift } from "@floating-ui/dom";

interface TooltipProps {
  label: string;
  children: ReactNode;
  side?: "top" | "bottom";
  // The wrap span is itself `position: relative` — pass a class here (rather
  // than styling the child button directly) when a caller needs to position
  // this whole tooltip+trigger unit against an ancestor, e.g. absolutely
  // placed over a thumbnail corner.
  className?: string;
}

// `side` is only the *preferred* placement — flip()/shift() push the
// tooltip off that edge and back into the viewport when the trigger sits
// near a corner (the bar's rightmost icons, thumbnail corners, ...), which
// a pure-CSS "always centered above" tooltip can't do without clipping.
export function Tooltip({ label, children, side = "top", className }: TooltipProps) {
  const wrapRef = useRef<HTMLSpanElement>(null);
  const tipRef = useRef<HTMLSpanElement>(null);
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);

  function show() {
    if (!wrapRef.current || !tipRef.current) return;
    computePosition(wrapRef.current, tipRef.current, {
      strategy: "fixed",
      placement: side,
      middleware: [offset(6), flip(), shift({ padding: 8 })]
    }).then(({ x, y }) => setPos({ x, y }));
  }

  function hide() {
    setPos(null);
  }

  return (
    <span
      ref={wrapRef}
      className={`fm-tooltip-wrap ${className ?? ""}`}
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocus={show}
      onBlur={hide}
    >
      {children}
      <span
        ref={tipRef}
        className={`fm-tooltip ${pos ? "fm-tooltip-visible" : ""}`}
        style={pos ? { left: pos.x, top: pos.y } : undefined}
        role="tooltip"
      >
        {label}
      </span>
    </span>
  );
}

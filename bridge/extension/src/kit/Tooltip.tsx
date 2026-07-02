import type { ReactNode } from "react";

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

// CSS-only (:hover/:focus-within) so it works for disabled buttons too —
// no JS positioning, no portal, safe inside the shadow-root content script.
export function Tooltip({ label, children, side = "top", className }: TooltipProps) {
  return (
    <span className={`fm-tooltip-wrap fm-tooltip-${side} ${className ?? ""}`}>
      {children}
      <span className="fm-tooltip" role="tooltip">
        {label}
      </span>
    </span>
  );
}

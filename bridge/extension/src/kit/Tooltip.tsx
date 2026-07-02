import type { ReactNode } from "react";

interface TooltipProps {
  label: string;
  children: ReactNode;
  side?: "top" | "bottom";
}

// CSS-only (:hover/:focus-within) so it works for disabled buttons too —
// no JS positioning, no portal, safe inside the shadow-root content script.
export function Tooltip({ label, children, side = "top" }: TooltipProps) {
  return (
    <span className={`fm-tooltip-wrap fm-tooltip-${side}`}>
      {children}
      <span className="fm-tooltip" role="tooltip">
        {label}
      </span>
    </span>
  );
}

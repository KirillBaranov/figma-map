import { Blend, Check, Crosshair, Inbox, Pin, RefreshCw, Settings, X } from "lucide-react";
import type { LucideProps } from "lucide-react";

// Thin re-exports so call sites (Bar, Window, IssuesWindow, ...) depend on
// kit/icons, not on lucide-react directly — the icon source stays kit's
// implementation detail, same as every other kit primitive.
export function CrosshairIcon(props: LucideProps) {
  return <Crosshair size={20} {...props} />;
}

export function CompareIcon(props: LucideProps) {
  return <Blend size={20} {...props} />;
}

export function InboxIcon(props: LucideProps) {
  return <Inbox size={20} {...props} />;
}

export function SettingsIcon(props: LucideProps) {
  return <Settings size={20} {...props} />;
}

export function CloseIcon(props: LucideProps) {
  return <X size={16} {...props} />;
}

export function CheckIcon(props: LucideProps) {
  return <Check size={16} {...props} />;
}

export function RefreshIcon(props: LucideProps) {
  return <RefreshCw size={16} {...props} />;
}

// Pass fill="currentColor" for the pinned (filled) state.
export function PinIcon(props: LucideProps) {
  return <Pin size={13} {...props} />;
}

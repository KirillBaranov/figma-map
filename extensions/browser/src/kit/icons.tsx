import { Blend, Check, ChevronDown, Crosshair, Eye, EyeOff, Inbox, Link2, Minus, Pin, Plus, RefreshCw, RotateCcw, Settings, Trash2, Upload, X } from "lucide-react";
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

export function PlusIcon(props: LucideProps) {
  return <Plus size={22} {...props} />;
}

export function EyeIcon(props: LucideProps) {
  return <Eye size={14} {...props} />;
}

export function EyeOffIcon(props: LucideProps) {
  return <EyeOff size={14} {...props} />;
}

export function SyncIcon(props: LucideProps) {
  return <Link2 size={14} {...props} />;
}

export function ResetIcon(props: LucideProps) {
  return <RotateCcw size={14} {...props} />;
}

export function ReplaceIcon(props: LucideProps) {
  return <Upload size={14} {...props} />;
}

// Minimize (collapse to title bar). ChevronDownIcon is its "restore"
// counterpart, shown on the collapsed title bar in place of this one.
export function MinimizeIcon(props: LucideProps) {
  return <Minus size={16} {...props} />;
}

export function ChevronDownIcon(props: LucideProps) {
  return <ChevronDown size={16} {...props} />;
}

export function TrashIcon(props: LucideProps) {
  return <Trash2 size={16} {...props} />;
}

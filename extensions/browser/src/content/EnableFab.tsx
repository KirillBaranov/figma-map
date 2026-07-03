import { PlusIcon, Tooltip } from "../kit";

interface EnableFabProps {
  onEnable: () => void;
}

// Default state on a site that hasn't been enabled — just this, not the
// full bar, so the tool doesn't clutter pages it isn't being used on.
// Clicking it adds the current host to the allowlist (lib/options.ts);
// SiteGate swaps in the full bar once that lands.
export function EnableFab({ onEnable }: EnableFabProps) {
  return (
    <Tooltip label="Enable figma-map on this site" className="fm-enable-fab-wrap">
      <button type="button" className="fm-enable-fab" onClick={onEnable} aria-label="Enable figma-map on this site">
        <PlusIcon />
      </button>
    </Tooltip>
  );
}

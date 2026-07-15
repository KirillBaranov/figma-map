export const DEFAULT_BRIDGE_URL = "http://localhost:1994";

export interface BarPos {
  x: number;
  y: number;
}

export interface ExtensionOptions {
  bridgeUrl: string;
  figmaNodeId?: string;
  fileKey?: string;
  // Sites the full bar is shown on — everywhere else the extension has zero
  // on-page footprint. Toggled from the toolbar popup (popup/App.tsx), never
  // from an on-page control.
  enabledHosts: string[];
  // Dragged-to position for the fixed bottom bar (Bar.tsx) — null means
  // "use the default bottom-center CSS position". A global preference, not
  // per-host: unlike enabledHosts, where you'd want the bar out of the way
  // is the same everywhere you use it.
  barPos: BarPos | null;
}

const KEYS: (keyof ExtensionOptions)[] = ["bridgeUrl", "figmaNodeId", "fileKey", "enabledHosts", "barPos"];

export async function getOptions(): Promise<ExtensionOptions> {
  const stored = await chrome.storage.sync.get(KEYS);
  return {
    bridgeUrl: stored.bridgeUrl || DEFAULT_BRIDGE_URL,
    figmaNodeId: stored.figmaNodeId || undefined,
    fileKey: stored.fileKey || undefined,
    enabledHosts: Array.isArray(stored.enabledHosts) ? stored.enabledHosts : [],
    barPos: stored.barPos ?? null
  };
}

export async function setOptions(options: ExtensionOptions): Promise<void> {
  await chrome.storage.sync.set(options);
}

export async function setBarPos(pos: BarPos | null): Promise<void> {
  const options = await getOptions();
  await setOptions({ ...options, barPos: pos });
}

export function isHostEnabled(hostname: string, options: ExtensionOptions): boolean {
  return options.enabledHosts.includes(hostname);
}

export async function setHostEnabled(hostname: string, enabled: boolean): Promise<void> {
  const options = await getOptions();
  const set = new Set(options.enabledHosts);
  if (enabled) {
    set.add(hostname);
  } else {
    set.delete(hostname);
  }
  await setOptions({ ...options, enabledHosts: Array.from(set) });
}

export const DEFAULT_BRIDGE_URL = "http://localhost:1994";

export interface ExtensionOptions {
  bridgeUrl: string;
  figmaNodeId?: string;
  fileKey?: string;
  // Sites the full bar is shown on — everywhere else gets only a small "+"
  // (EnableFab) so the tool doesn't clutter pages it isn't being used on.
  enabledHosts: string[];
}

const KEYS: (keyof ExtensionOptions)[] = ["bridgeUrl", "figmaNodeId", "fileKey", "enabledHosts"];

export async function getOptions(): Promise<ExtensionOptions> {
  const stored = await chrome.storage.sync.get(KEYS);
  return {
    bridgeUrl: stored.bridgeUrl || DEFAULT_BRIDGE_URL,
    figmaNodeId: stored.figmaNodeId || undefined,
    fileKey: stored.fileKey || undefined,
    enabledHosts: Array.isArray(stored.enabledHosts) ? stored.enabledHosts : []
  };
}

export async function setOptions(options: ExtensionOptions): Promise<void> {
  await chrome.storage.sync.set(options);
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

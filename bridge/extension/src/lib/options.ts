export const DEFAULT_BRIDGE_URL = "http://localhost:1994";

export interface ExtensionOptions {
  bridgeUrl: string;
  figmaNodeId?: string;
  fileKey?: string;
}

const KEYS: (keyof ExtensionOptions)[] = ["bridgeUrl", "figmaNodeId", "fileKey"];

export async function getOptions(): Promise<ExtensionOptions> {
  const stored = await chrome.storage.sync.get(KEYS);
  return {
    bridgeUrl: stored.bridgeUrl || DEFAULT_BRIDGE_URL,
    figmaNodeId: stored.figmaNodeId || undefined,
    fileKey: stored.fileKey || undefined
  };
}

export async function setOptions(options: ExtensionOptions): Promise<void> {
  await chrome.storage.sync.set(options);
}

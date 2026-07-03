import { useEffect, useState, type Ref } from "react";
import { App, type AppHandle } from "./App";
import { EnableFab } from "./EnableFab";
import { setHostEnabled } from "../lib/options";

interface SiteGateProps {
  initialEnabled: boolean;
  hostname: string;
  appRef: Ref<AppHandle>;
  onToggleSelect: () => void;
  onStopSelect: () => void;
  onOpenSettings: () => void;
}

// Gates the full bar behind the per-site allowlist (lib/options.ts). Reacts
// to chrome.storage changes so toggling from the popup takes effect
// immediately, in either direction, without a page reload.
export function SiteGate({ initialEnabled, hostname, appRef, onToggleSelect, onStopSelect, onOpenSettings }: SiteGateProps) {
  const [enabled, setEnabled] = useState(initialEnabled);

  useEffect(() => {
    function onChanged(changes: Record<string, chrome.storage.StorageChange>, area: string) {
      if (area !== "sync" || !changes.enabledHosts) return;
      const hosts = (changes.enabledHosts.newValue as string[] | undefined) ?? [];
      setEnabled(hosts.includes(hostname));
    }
    chrome.storage.onChanged.addListener(onChanged);
    return () => chrome.storage.onChanged.removeListener(onChanged);
  }, [hostname]);

  if (!enabled) {
    return <EnableFab onEnable={() => setHostEnabled(hostname, true)} />;
  }

  return <App ref={appRef} onToggleSelect={onToggleSelect} onStopSelect={onStopSelect} onOpenSettings={onOpenSettings} />;
}

import { useEffect, useState } from "react";
import { Button, StatusBadge } from "../kit";
import { getStatus } from "../lib/status";

export function App() {
  const [status, setStatus] = useState<"pending" | "connected" | "disconnected">("pending");
  const [pending, setPending] = useState<number | null>(null);
  const [selecting, setSelecting] = useState(false);

  useEffect(() => {
    getStatus().then(({ connected, pending }) => {
      setStatus(connected ? "connected" : "disconnected");
      setPending(pending);
    });
  }, []);

  async function toggleSelect() {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (!tab?.id) return;
    const resp = await chrome.tabs.sendMessage(tab.id, { type: "FIGMA_MAP_TOGGLE_SELECT" });
    setSelecting(Boolean(resp?.selecting));
    window.close();
  }

  return (
    <div className="fm-reset fm-popup">
      <h1>figma-map Capture</h1>
      <StatusBadge status={status} />
      {pending !== null && pending > 0 && (
        <div className="fm-popup-meta">{pending} pending issue{pending === 1 ? "" : "s"}</div>
      )}
      <Button onClick={toggleSelect} disabled={status === "disconnected"}>
        {selecting ? "Selecting… (Esc to cancel)" : "Select region"}
      </Button>
      <div className="fm-popup-footer">
        <span className="fm-popup-meta">v0.1.0</span>
        <Button variant="ghost" onClick={() => chrome.runtime.openOptionsPage()}>
          Settings
        </Button>
      </div>
    </div>
  );
}

import { forwardRef, useEffect, useImperativeHandle, useState } from "react";
import { Button, TextAreaField, Toast, useToast } from "../kit";
import { Bar } from "./Bar";
import { CompareOverlay } from "./CompareOverlay";
import { IssuesWindow } from "./IssuesWindow";
import { getStatus } from "../lib/status";
import { getOptions } from "../lib/options";

export interface AppHandle {
  showPanel(opts: { selector: string; x: number; y: number; onSend: (note: string) => void }): void;
  hidePanel(): void;
  showToast(message: string): void;
  setSelecting(value: boolean): void;
}

interface PanelState {
  selector: string;
  x: number;
  y: number;
  onSend: (note: string) => void;
}

type ActiveWindow = "compare" | "issues" | null;

interface AppProps {
  onToggleSelect: () => void;
  onStopSelect: () => void;
  onOpenSettings: () => void;
}

const STATUS_REFRESH_MS = 30_000;

export const App = forwardRef<AppHandle, AppProps>(function App({ onToggleSelect, onStopSelect, onOpenSettings }, ref) {
  const [status, setStatus] = useState<"pending" | "connected" | "disconnected">("pending");
  const [pending, setPending] = useState<number | null>(null);
  const [selecting, setSelecting] = useState(false);
  const [panel, setPanel] = useState<PanelState | null>(null);
  const [note, setNote] = useState("");
  const [activeWindow, setActiveWindow] = useState<ActiveWindow>(null);
  const [defaultFigmaNodeId, setDefaultFigmaNodeId] = useState<string | undefined>();
  const [defaultFileKey, setDefaultFileKey] = useState<string | undefined>();
  const toast = useToast();

  useEffect(() => {
    async function refresh() {
      const { connected, pending } = await getStatus();
      setStatus(connected ? "connected" : "disconnected");
      setPending(pending);
    }
    refresh();
    const id = setInterval(refresh, STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    getOptions().then((opts) => {
      setDefaultFigmaNodeId(opts.figmaNodeId);
      setDefaultFileKey(opts.fileKey);
    });
  }, []);

  useImperativeHandle(ref, () => ({
    showPanel(opts) {
      setNote("");
      setPanel(opts);
    },
    hidePanel() {
      setPanel(null);
    },
    showToast(message) {
      toast.show(message);
    },
    setSelecting
  }));

  // Only one of {Select region, Overlay compare, Issues} active at a time —
  // opening one closes/cancels whichever was open.
  function handleToggleSelect() {
    setActiveWindow(null);
    onToggleSelect();
  }

  function handleToggleWindow(target: Exclude<ActiveWindow, null>) {
    if (selecting) onStopSelect();
    setActiveWindow((current) => (current === target ? null : target));
  }

  return (
    <>
      <Bar
        status={status}
        pending={pending}
        selecting={selecting}
        activeWindow={activeWindow}
        onToggleSelect={handleToggleSelect}
        onToggleCompare={() => handleToggleWindow("compare")}
        onToggleIssues={() => handleToggleWindow("issues")}
        onOpenSettings={onOpenSettings}
      />
      {activeWindow === "compare" && (
        <CompareOverlay
          defaultNodeId={defaultFigmaNodeId}
          defaultFileKey={defaultFileKey}
          onClose={() => setActiveWindow(null)}
        />
      )}
      {activeWindow === "issues" && (
        <IssuesWindow fileKey={defaultFileKey} onClose={() => setActiveWindow(null)} />
      )}
      {panel && (
        <div className="fm-reset fm-confirm-panel" style={{ left: panel.x, top: panel.y }}>
          <div className="fm-confirm-selector">{panel.selector}</div>
          <TextAreaField
            label="Note for the agent"
            placeholder="e.g. 'spacing looks off vs Figma'"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            autoFocus
          />
          <div className="fm-confirm-actions">
            <Button variant="secondary" onClick={() => setPanel(null)}>
              Cancel
            </Button>
            <Button
              onClick={() => {
                panel.onSend(note);
                setPanel(null);
              }}
            >
              Send
            </Button>
          </div>
        </div>
      )}
      <div className="fm-reset">
        <Toast message={toast.message} />
      </div>
    </>
  );
});

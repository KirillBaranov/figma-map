import { Button, TextAreaField, TextField } from "../kit";
import { CompareHistoryRibbon } from "./CompareHistoryRibbon";
import type { CompareHistoryEntryData } from "../lib/compareSession";
import type { Size } from "./hooks/useCompareState";

interface ComparePanelProps {
  onSelectHistory: (entry: CompareHistoryEntryData) => void;
  image: string | null;
  nodeId: string;
  setNodeId: (v: string) => void;
  fetching: boolean;
  onFetchFromFigma: () => void;
  selecting: boolean;
  onUseFigmaSelection: () => void;
  onPasteFromClipboard: () => void;

  figmaSize: Size | null;
  viewportWidth: number;
  zooming: boolean;
  onMatchZoom: () => void;

  opacity: number;
  setOpacity: (v: number) => void;
  scale: number;
  setScale: (v: number) => void;
  hidden: boolean;
  onToggleHidden: () => void;
  diffMode: boolean;
  onToggleDiffMode: () => void;
  syncScroll: boolean;
  onToggleSyncScroll: () => void;
  onResetView: () => void;
  onReplace: () => void;

  note: string;
  setNote: (v: string) => void;
  sending: boolean;
  sent: boolean;
  onSendToAgent: () => void;

  fetchedNodeId: string | null;
  hasHitTree: boolean;
  regionLabel: string | null;

  error: string | null;
}

// Pure content for the "Overlay compare" window — Figma fetch/paste/
// selection entry points before an image is loaded, then opacity/scale/
// diff/sync controls and the "send to agent" form once one is. The frame
// (drag, close, drop/paste target) is owned by <Window>, not this component.
export function ComparePanel({
  onSelectHistory,
  image,
  nodeId,
  setNodeId,
  fetching,
  onFetchFromFigma,
  selecting,
  onUseFigmaSelection,
  onPasteFromClipboard,
  figmaSize,
  viewportWidth,
  zooming,
  onMatchZoom,
  opacity,
  setOpacity,
  scale,
  setScale,
  hidden,
  onToggleHidden,
  diffMode,
  onToggleDiffMode,
  syncScroll,
  onToggleSyncScroll,
  onResetView,
  onReplace,
  note,
  setNote,
  sending,
  sent,
  onSendToAgent,
  fetchedNodeId,
  hasHitTree,
  regionLabel,
  error
}: ComparePanelProps) {
  return (
    <>
      <CompareHistoryRibbon onSelect={onSelectHistory} />
      {!image && (
        <>
          <TextField
            label="Figma node id"
            placeholder="123:456"
            value={nodeId}
            onChange={(e) => setNodeId(e.target.value)}
          />
          <Button onClick={onFetchFromFigma} disabled={fetching}>
            {fetching ? "Fetching…" : "Fetch from Figma"}
          </Button>
          <Button variant="secondary" onClick={onUseFigmaSelection} disabled={selecting}>
            {selecting ? "Reading selection…" : "Use Figma selection"}
          </Button>
          <Button variant="secondary" onClick={onPasteFromClipboard}>
            Paste from clipboard
          </Button>
          <div className="fm-compare-hint">or drop an image here</div>
        </>
      )}

      {image && (
        <>
          {figmaSize && (
            <div className="fm-compare-zoom">
              <div className="fm-range-label">
                <span>Viewport</span>
                <span className={viewportWidth === figmaSize.w ? "fm-compare-match" : ""}>
                  {viewportWidth}px / {figmaSize.w}px
                </span>
              </div>
              <Button variant="secondary" onClick={onMatchZoom} disabled={zooming}>
                {zooming ? "Setting zoom…" : `Match zoom to ${figmaSize.w}px`}
              </Button>
            </div>
          )}
          <div className="fm-range-label">
            <span>Opacity</span>
            <span>{opacity}%</span>
          </div>
          <input
            className="fm-range"
            type="range"
            min={0}
            max={100}
            value={opacity}
            onChange={(e) => setOpacity(Number(e.target.value))}
          />
          <div className="fm-range-label">
            <span>Scale</span>
            <span>{scale}%</span>
          </div>
          <input
            className="fm-range"
            type="range"
            min={25}
            max={200}
            value={scale}
            onChange={(e) => setScale(Number(e.target.value))}
          />
          <div className="fm-compare-actions">
            <Button variant="secondary" onClick={onToggleHidden}>
              {hidden ? "Show" : "Hide"}
            </Button>
            <Button variant={diffMode ? "primary" : "secondary"} onClick={onToggleDiffMode}>
              Diff mode
            </Button>
          </div>
          <Button variant={syncScroll ? "primary" : "secondary"} onClick={onToggleSyncScroll}>
            {syncScroll ? "Sync scroll: on" : "Sync scroll: off"}
          </Button>
          <div className="fm-compare-hint">↑↓←→ nudge 1px · Shift = 10px</div>
          <div className="fm-compare-actions">
            <Button variant="secondary" onClick={onResetView}>
              Reset
            </Button>
            <Button variant="secondary" onClick={onReplace}>
              Replace
            </Button>
          </div>
          <TextAreaField
            label="Note for the agent (optional)"
            placeholder="e.g. 'aligned, but title block sits ~2px low'"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
          <Button onClick={onSendToAgent} disabled={sending}>
            {sending ? "Sending…" : sent ? "Sent ✓" : "Send to agent"}
          </Button>
          {fetchedNodeId && <div className="fm-compare-hint">linked to Figma node {fetchedNodeId}</div>}
          {hasHitTree && (
            <div className="fm-compare-hint">
              {regionLabel ? `pinned: ${regionLabel}` : "Alt+click the overlay to pin a region"}
            </div>
          )}
        </>
      )}

      {error && <div className="fm-compare-error">{error}</div>}
    </>
  );
}

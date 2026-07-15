import { useState } from "react";
import { Button, ChevronDownIcon, RefreshIcon, ReplaceIcon, ResetIcon, TextAreaField, TextField } from "../kit";
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
  diffContrast: boolean;
  onCycleDiffMode: () => void;
  diffComputing: boolean;
  onRefreshDiff: () => void;
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
  diffContrast,
  onCycleDiffMode,
  diffComputing,
  onRefreshDiff,
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
  const [advancedOpen, setAdvancedOpen] = useState(false);
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
          <div className="fm-section-header">
            <span>Align</span>
            <div className="fm-icon-actions">
              <button className="fm-icon-btn-bare" onClick={onResetView} title="Recenter position and reset opacity (keeps scale)">
                <ResetIcon />
              </button>
              <button className="fm-icon-btn-bare" onClick={onReplace} title="Replace reference image">
                <ReplaceIcon />
              </button>
            </div>
          </div>
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
          <div className="fm-compare-hint">↑↓←→ nudge 1px · Shift = 10px</div>

          <div className="fm-section-header">
            <span>Compare</span>
          </div>
          <div className="fm-compare-actions">
            <Button variant={hidden ? "primary" : "secondary"} onClick={onToggleHidden}>
              {hidden ? "Hidden" : "Visible"}
            </Button>
            <Button variant={diffMode ? "primary" : "secondary"} onClick={onCycleDiffMode}>
              {!diffMode ? "Diff mode" : diffContrast ? "Diff: Contrast" : "Diff: Blend"}
            </Button>
            {diffMode && diffContrast && (
              <button
                className="fm-icon-btn-bare"
                onClick={onRefreshDiff}
                disabled={diffComputing}
                title="Re-capture and re-diff"
              >
                <RefreshIcon className={diffComputing ? "fm-spin" : undefined} />
              </button>
            )}
          </div>

          <button
            type="button"
            className="fm-section-header fm-section-header-toggle"
            onClick={() => setAdvancedOpen((v) => !v)}
          >
            <span>Advanced</span>
            <ChevronDownIcon className={advancedOpen ? "fm-chevron-open" : undefined} />
          </button>
          {advancedOpen && (
            <>
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
            </>
          )}

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

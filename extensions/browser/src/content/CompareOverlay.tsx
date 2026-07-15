import { useCompareState } from "./hooks/useCompareState";
import { useFigmaCompare } from "./hooks/useFigmaCompare";
import { useDiffSnapshot } from "./hooks/useDiffSnapshot";
import { useDraggable } from "./hooks/useDraggable";
import { CompareImage } from "./CompareImage";
import { ComparePanel } from "./ComparePanel";
import { Window } from "./Window";

interface CompareOverlayProps {
  defaultNodeId?: string;
  defaultFileKey?: string;
  onClose: () => void;
}

// View + composition only: all state lives in useCompareState, all
// Figma/clipboard/drop data flow lives in useFigmaCompare. This component
// wires the two together, the drag mechanics (useDraggable) to
// CompareImage, and the window frame (Window) around ComparePanel's content.
export function CompareOverlay({ defaultNodeId, defaultFileKey, onClose }: CompareOverlayProps) {
  const state = useCompareState(defaultNodeId);
  const actions = useFigmaCompare(state, defaultFileKey);
  const diffSnapshot = useDiffSnapshot(state);

  const imageDrag = useDraggable(state.setPos, state.pos);

  function onImageMouseDown(e: React.MouseEvent) {
    // Alt+click pins a region for the agent instead of dragging the
    // overlay — only meaningful once a hit-map is available (selection mode).
    if (state.hitTree && e.altKey) {
      actions.resolveClickToRegion(e);
      return;
    }
    imageDrag.startDrag(e);
  }

  return (
    <>
      <CompareImage
        image={state.image}
        naturalSize={state.naturalSize}
        pos={state.pos}
        scale={state.scale}
        opacity={state.opacity}
        hidden={state.hidden}
        diffMode={state.diffMode}
        diffContrast={state.diffContrast}
        diffSrc={diffSnapshot.diffSrc}
        syncScroll={state.syncScroll}
        dragging={imageDrag.dragging}
        onImageMouseDown={onImageMouseDown}
        region={state.region}
        rootBounds={state.rootBounds}
      />
      <Window
        title="Overlay compare"
        pos={state.panelPos}
        onPosChange={state.setPanelPos}
        onClose={onClose}
        draggingOver={state.draggingOver}
        onDragOver={(e) => {
          e.preventDefault();
          state.setDraggingOver(true);
        }}
        onDragLeave={() => state.setDraggingOver(false)}
        onDrop={actions.onDrop}
        onPaste={actions.onPaste}
      >
        <ComparePanel
          onSelectHistory={actions.loadFromHistory}
          image={state.image}
          nodeId={state.nodeId}
          setNodeId={state.setNodeId}
          fetching={state.fetching}
          onFetchFromFigma={actions.fetchFromFigma}
          selecting={state.selecting}
          onUseFigmaSelection={actions.fetchFigmaSelection}
          onPasteFromClipboard={actions.pasteFromClipboard}
          figmaSize={state.figmaSize}
          viewportWidth={state.viewportWidth}
          zooming={state.zooming}
          onMatchZoom={actions.matchZoom}
          opacity={state.opacity}
          setOpacity={state.setOpacity}
          scale={state.scale}
          setScale={state.setScale}
          hidden={state.hidden}
          onToggleHidden={() => state.setHidden((h) => !h)}
          diffMode={state.diffMode}
          diffContrast={state.diffContrast}
          onCycleDiffMode={actions.cycleDiffMode}
          diffComputing={diffSnapshot.computing}
          onRefreshDiff={diffSnapshot.recompute}
          onResetView={actions.resetView}
          onReplace={state.clearImage}
          note={state.note}
          setNote={state.setNote}
          sending={state.sending}
          sent={state.sent}
          onSendToAgent={actions.sendToAgent}
          fetchedNodeId={state.fetchedNodeId}
          hasHitTree={!!state.hitTree}
          regionLabel={state.region ? `${state.region.name} (${state.region.id})` : null}
          error={state.error}
        />
      </Window>
    </>
  );
}

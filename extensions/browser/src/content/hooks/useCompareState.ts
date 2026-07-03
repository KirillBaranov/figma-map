import { useEffect, useState } from "react";
import type { HitBounds, HitNode } from "../../lib/hitmap";
import { clearCompareState, loadCompareState, saveCompareState } from "../../lib/compareSession";
import type { Pos } from "./useDraggable";

export interface Size {
  w: number;
  h: number;
}

export interface RegionPin {
  id: string;
  name: string;
  bounds: HitBounds;
}

export function centerPos(w: number, h: number): Pos {
  return {
    x: Math.max(0, Math.round((window.innerWidth - w) / 2)),
    y: Math.max(0, Math.round((window.innerHeight - h) / 2))
  };
}

// Owns every piece of CompareOverlay's state plus the effects that persist
// it (restore-on-mount, debounced save, clear) and the two effects that
// track ambient browser state (viewport width, arrow-key nudge). Pure state
// — no Figma/network calls; those live in useFigmaCompare, which takes this
// hook's setters as input.
export function useCompareState(defaultNodeId?: string) {
  const [nodeId, setNodeId] = useState(defaultNodeId ?? "");
  const [image, setImage] = useState<string | null>(null);
  const [naturalSize, setNaturalSize] = useState<Size | null>(null);
  const [opacity, setOpacity] = useState(70);
  const [scale, setScale] = useState(100);
  const [pos, setPos] = useState<Pos>({ x: 80, y: 80 });
  const [hidden, setHidden] = useState(false);
  const [diffMode, setDiffMode] = useState(false);
  const [fetching, setFetching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [draggingOver, setDraggingOver] = useState(false);
  const [figmaSize, setFigmaSize] = useState<Size | null>(null);
  const [fetchedNodeId, setFetchedNodeId] = useState<string | null>(null);
  // Selection mode: hitTree is the fetched node's subtree (bounds +
  // children), rootBounds anchors it in absolute Figma canvas coordinates so
  // overlay clicks can be mapped back onto it. Both are session-only — a
  // subtree goes stale the moment the user edits Figma, so we always refetch
  // rather than persisting it to compareStorage.
  const [hitTree, setHitTree] = useState<HitNode | null>(null);
  const [rootBounds, setRootBounds] = useState<HitBounds | null>(null);
  const [region, setRegion] = useState<RegionPin | null>(null);
  const [selecting, setSelecting] = useState(false);
  const [viewportWidth, setViewportWidth] = useState(window.innerWidth);
  const [zooming, setZooming] = useState(false);
  const [note, setNote] = useState("");
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState(false);
  const [syncScroll, setSyncScroll] = useState(false);
  const [restored, setRestored] = useState(false);
  const [panelPos, setPanelPos] = useState<Pos | null>(null);

  // Restore whatever was last compared — surviving a page reload, not just
  // re-opening the panel, is the whole point: re-pasting/re-fetching the
  // same reference image on every refresh while iterating is the friction
  // this is meant to remove.
  useEffect(() => {
    loadCompareState().then((s) => {
      if (!s) {
        setRestored(true);
        return;
      }
      setImage(s.image);
      setNaturalSize({ w: s.naturalW, h: s.naturalH });
      if (s.figmaW && s.figmaH) setFigmaSize({ w: s.figmaW, h: s.figmaH });
      if (s.fetchedNodeId) setFetchedNodeId(s.fetchedNodeId);
      if (s.nodeId) setNodeId(s.nodeId);
      setPos(s.pos);
      setScale(s.scale);
      setOpacity(s.opacity);
      setDiffMode(s.diffMode);
      setSyncScroll(s.syncScroll);
      if (s.panelPos) setPanelPos(s.panelPos);
      setRestored(true);
    });
  }, []);

  // Debounced persist — drag/arrow-nudge can fire many updates a second,
  // no need to hit chrome.storage on every one of them.
  useEffect(() => {
    if (!restored || !image || !naturalSize) return;
    const id = setTimeout(() => {
      saveCompareState({
        image,
        naturalW: naturalSize.w,
        naturalH: naturalSize.h,
        figmaW: figmaSize?.w,
        figmaH: figmaSize?.h,
        fetchedNodeId: fetchedNodeId ?? undefined,
        nodeId,
        pos,
        scale,
        opacity,
        diffMode,
        syncScroll,
        panelPos: panelPos ?? undefined
      });
    }, 300);
    return () => clearTimeout(id);
  }, [restored, image, naturalSize, figmaSize, fetchedNodeId, nodeId, pos, scale, opacity, diffMode, syncScroll, panelPos]);

  // Live readout so a "design is wider than my screen" zoom mismatch is
  // visible as numbers, not just eyeballed — and updates as Match zoom (or
  // a manual Cmd+-/Cmd+=) changes it.
  useEffect(() => {
    function onResize() {
      setViewportWidth(window.innerWidth);
    }
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  // Arrow-key nudge for fine alignment (Figma-style: 1px, Shift = 10px).
  // Capture phase + stopPropagation so this wins over a focused range slider
  // (which otherwise eats arrow keys itself) and over the host page.
  useEffect(() => {
    if (!image) return;
    function onKeyDown(e: KeyboardEvent) {
      const step = e.shiftKey ? 10 : 1;
      let dx = 0;
      let dy = 0;
      switch (e.key) {
        case "ArrowUp":
          dy = -step;
          break;
        case "ArrowDown":
          dy = step;
          break;
        case "ArrowLeft":
          dx = -step;
          break;
        case "ArrowRight":
          dx = step;
          break;
        default:
          return;
      }
      e.preventDefault();
      e.stopPropagation();
      setPos((p) => ({ x: p.x + dx, y: p.y + dy }));
    }
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, [image]);

  // Off: the overlay is `position: fixed` — glued to the viewport, ignores
  // scroll (good while you're actively dragging it into place against
  // what's currently visible). On: `position: absolute` — anchored to the
  // document, so it scrolls together with the page underneath it. Convert
  // pos between the two coordinate spaces so the image doesn't jump when
  // toggling.
  function toggleSyncScroll() {
    const next = !syncScroll;
    setPos((p) =>
      next ? { x: p.x + window.scrollX, y: p.y + window.scrollY } : { x: p.x - window.scrollX, y: p.y - window.scrollY }
    );
    setSyncScroll(next);
  }

  function clearImage() {
    setImage(null);
    setNaturalSize(null);
    setFigmaSize(null);
    setFetchedNodeId(null);
    setDiffMode(false);
    setHitTree(null);
    setRootBounds(null);
    setRegion(null);
    clearCompareState();
  }

  return {
    nodeId, setNodeId,
    image, setImage,
    naturalSize, setNaturalSize,
    opacity, setOpacity,
    scale, setScale,
    pos, setPos,
    hidden, setHidden,
    diffMode, setDiffMode,
    fetching, setFetching,
    error, setError,
    draggingOver, setDraggingOver,
    figmaSize, setFigmaSize,
    fetchedNodeId, setFetchedNodeId,
    hitTree, setHitTree,
    rootBounds, setRootBounds,
    region, setRegion,
    selecting, setSelecting,
    viewportWidth,
    zooming, setZooming,
    note, setNote,
    sending, setSending,
    sent, setSent,
    syncScroll, toggleSyncScroll,
    panelPos, setPanelPos,
    clearImage
  };
}

export type CompareState = ReturnType<typeof useCompareState>;

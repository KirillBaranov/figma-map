import { useRef } from "react";
import {
  fetchFigmaScreenshot,
  getFigmaSelection,
  getFigmaSubtree,
  matchZoomToWidth,
  sendCompareToAgent
} from "../../lib/figma";
import { resolveHit } from "../../lib/hitmap";
import { pushCompareHistory, type CompareHistoryEntryData } from "../../lib/compareSession";
import type { CompareState } from "./useCompareState";
import { centerPos } from "./useCompareState";

function readImageBlob(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}

// The captured-screenshot "Contrast" diff renderer (useDiffSnapshot) is
// janky in practice — off until that's actually fixed, so cycleDiffMode
// below is a plain Off<->Blend toggle. Flip this back on to test it.
const CONTRAST_DIFF_ENABLED = false;

// Data orchestration for CompareOverlay: everything that talks to Figma (via
// lib/figma.ts), the clipboard, or dropped files, and everything derived
// from that (loading an image, resolving a click to a Figma node). Takes
// useCompareState's setters rather than owning any state itself.
export function useFigmaCompare(state: CompareState, defaultFileKey?: string) {
  const opacityBeforeDiff = useRef(70);

  function loadImage(dataUrl: string) {
    const img = new Image();
    img.onload = () => {
      state.setNaturalSize({ w: img.naturalWidth, h: img.naturalHeight });
      // Center by default — eyeballing alignment from a corner is needless
      // friction, especially at non-100% browser zoom where small offsets
      // are harder to judge.
      state.setPos(centerPos(img.naturalWidth, img.naturalHeight));
    };
    img.src = dataUrl;
    state.setImage(dataUrl);
    state.setError(null);
  }

  function enterDiffMode() {
    opacityBeforeDiff.current = state.opacity;
    state.setOpacity(100); // a partial-opacity diff is just a muddy blend, not a diff
    state.setDiffMode(true);
  }

  function exitDiffMode() {
    state.setOpacity(opacityBeforeDiff.current);
    state.setDiffMode(false);
    state.setDiffContrast(false);
  }

  // One button cycles Off -> Blend -> Contrast -> Off instead of a
  // Diff-mode toggle plus a separate Blend/Contrast picker — fewer controls
  // competing for space in an already button-dense panel. Off -> Blend ->
  // Off when CONTRAST_DIFF_ENABLED is false (the default).
  function cycleDiffMode() {
    if (!state.diffMode) {
      enterDiffMode(); // -> Blend
    } else if (CONTRAST_DIFF_ENABLED && !state.diffContrast) {
      state.setDiffContrast(true); // -> Contrast
    } else {
      exitDiffMode(); // -> Off
    }
  }

  // Recenters position (at whatever scale is currently set) and resets
  // opacity — deliberately leaves scale alone. Scale is usually a deliberate
  // calibration (e.g. via Match zoom), so wiping it back to 100% every time
  // someone just wants to recenter a dragged-off overlay was the wrong
  // default.
  function resetView() {
    state.setOpacity(state.diffMode ? 100 : 70);
    if (state.naturalSize) {
      const w = (state.naturalSize.w * state.scale) / 100;
      const h = (state.naturalSize.h * state.scale) / 100;
      state.setPos(centerPos(w, h));
    }
  }

  // Manual paste/drop have no Figma metadata and no synchronously-known
  // pixel size (only the Image().onload inside loadImage knows that) — push
  // a minimal snapshot. naturalW/H are recomputed fresh from the image on
  // reactivation anyway (loadFromHistory calls loadImage), so 0/0 here is
  // never actually read for anything but the ribbon's <img> thumbnail,
  // which is sized by CSS, not by this metadata.
  function pushManualHistory(dataUrl: string) {
    pushCompareHistory({
      image: dataUrl,
      naturalW: 0,
      naturalH: 0,
      nodeId: "",
      pos: { x: 0, y: 0 },
      scale: 100,
      opacity: 70,
      diffMode: false,
      syncScroll: false
    }).catch(() => {});
  }

  function onPaste(e: React.ClipboardEvent) {
    const item = Array.from(e.clipboardData.items).find((i) => i.type.startsWith("image/"));
    const file = item?.getAsFile();
    if (!file) return;
    e.preventDefault();
    readImageBlob(file).then((dataUrl) => {
      loadImage(dataUrl);
      pushManualHistory(dataUrl);
    });
  }

  async function pasteFromClipboard() {
    state.setError(null);
    try {
      const items = await navigator.clipboard.read();
      for (const item of items) {
        const type = item.types.find((t) => t.startsWith("image/"));
        if (!type) continue;
        const blob = await item.getType(type);
        const dataUrl = await readImageBlob(blob);
        loadImage(dataUrl);
        pushManualHistory(dataUrl);
        return;
      }
      const foundTypes = items.flatMap((i) => i.types);
      state.setError(
        foundTypes.length
          ? `Clipboard has ${foundTypes.join(", ")} but no image — in Figma, right-click the layer → "Copy/Paste as" → "Copy as PNG" (plain ⌘C doesn't put a raster image on the system clipboard)`
          : "Clipboard is empty"
      );
    } catch (err) {
      state.setError(err instanceof Error ? err.message : String(err));
    }
  }

  function onDrop(e: React.DragEvent) {
    e.preventDefault();
    state.setDraggingOver(false);
    const file = e.dataTransfer.files[0];
    if (!file) return;
    readImageBlob(file).then((dataUrl) => {
      loadImage(dataUrl);
      pushManualHistory(dataUrl);
    });
  }

  async function fetchFromFigma() {
    const nodeId = state.nodeId.trim();
    if (!nodeId) {
      state.setError("Enter a Figma node id");
      return;
    }
    state.setFetching(true);
    state.setError(null);
    try {
      // Fetch the subtree alongside the screenshot — same as "Use Figma
      // selection" — so Alt+click-to-pin-a-region works no matter which of
      // the three ways (typed id, live selection, history) loaded the node.
      const [result, subtree] = await Promise.all([
        fetchFigmaScreenshot(nodeId, defaultFileKey),
        getFigmaSubtree(nodeId, defaultFileKey)
      ]);
      state.setFigmaSize({ w: result.width, h: result.height });
      state.setFetchedNodeId(nodeId);
      state.setRootBounds(subtree.bounds);
      state.setHitTree(subtree);
      state.setRegion(null);
      loadImage(result.dataUrl);
      pushCompareHistory({
        image: result.dataUrl,
        naturalW: result.width,
        naturalH: result.height,
        figmaW: result.width,
        figmaH: result.height,
        fetchedNodeId: nodeId,
        nodeId,
        pos: { x: 0, y: 0 },
        scale: 100,
        opacity: 70,
        diffMode: false,
        syncScroll: false
      }).catch(() => {});
    } catch (err) {
      state.setError(err instanceof Error ? err.message : String(err));
    } finally {
      state.setFetching(false);
    }
  }

  // "Cool mode": pulls the image straight from whatever's selected live in
  // Figma right now, plus its subtree (bounds + children) so clicking the
  // overlay can resolve a specific descendant instead of just the root node.
  async function fetchFigmaSelection() {
    state.setSelecting(true);
    state.setError(null);
    try {
      const selected = await getFigmaSelection(defaultFileKey);
      const node = selected[0];
      if (!node) {
        throw new Error("nothing selected — select a layer in Figma and try again");
      }
      const [result, subtree] = await Promise.all([
        fetchFigmaScreenshot(node.id, defaultFileKey),
        getFigmaSubtree(node.id, defaultFileKey)
      ]);
      state.setFigmaSize({ w: result.width, h: result.height });
      state.setFetchedNodeId(node.id);
      state.setNodeId(node.id);
      state.setRootBounds(node.bounds);
      state.setHitTree(subtree);
      state.setRegion(null);
      loadImage(result.dataUrl);
      pushCompareHistory({
        image: result.dataUrl,
        naturalW: result.width,
        naturalH: result.height,
        figmaW: result.width,
        figmaH: result.height,
        fetchedNodeId: node.id,
        nodeId: node.id,
        pos: { x: 0, y: 0 },
        scale: 100,
        opacity: 70,
        diffMode: false,
        syncScroll: false
      }).catch(() => {});
    } catch (err) {
      state.setError(err instanceof Error ? err.message : String(err));
    } finally {
      state.setSelecting(false);
    }
  }

  // Maps a click on the overlay image to absolute Figma canvas coordinates
  // (the image was fetched 1:1 against rootBounds, so this is a direct
  // affine transform) and resolves it against hitTree.
  function resolveClickToRegion(e: React.MouseEvent) {
    if (!state.hitTree || !state.rootBounds) return;
    const factor = 100 / state.scale;
    const figmaX = state.rootBounds.x + (e.clientX - state.pos.x) * factor;
    const figmaY = state.rootBounds.y + (e.clientY - state.pos.y) * factor;
    const hit = resolveHit(state.hitTree, { x: figmaX, y: figmaY });
    state.setRegion(hit ? { id: hit.id, name: hit.name, bounds: hit.bounds } : null);
  }

  // E.g. a 1920px-wide design viewed on a 1440px laptop: this sets the
  // browser's zoom so window.innerWidth becomes exactly figmaSize.w, instead
  // of guessing a zoom % by eye and being off by a handful of CSS px.
  async function matchZoom() {
    if (!state.figmaSize) return;
    state.setZooming(true);
    state.setError(null);
    try {
      await matchZoomToWidth(state.figmaSize.w);
    } catch (err) {
      state.setError(err instanceof Error ? err.message : String(err));
    } finally {
      state.setZooming(false);
    }
  }

  // Ships exactly the rect the user dragged the overlay to — i.e. the
  // region they already hand-aligned against the Figma render — as a
  // flagged issue. With figmaNodeId attached, the agent can re-fetch the
  // same Figma render and run `verify pixeldiff-images` for a measured
  // diff%, instead of being told "looks off" with no numbers.
  async function sendToAgent() {
    if (!state.naturalSize) return;
    state.setSending(true);
    state.setError(null);
    try {
      await sendCompareToAgent({
        bbox: {
          x: state.pos.x,
          y: state.pos.y,
          width: (state.naturalSize.w * state.scale) / 100,
          height: (state.naturalSize.h * state.scale) / 100
        },
        dpr: window.devicePixelRatio || 1,
        tabUrl: location.href,
        note: state.note || undefined,
        figmaNodeId: state.fetchedNodeId ?? undefined,
        regionNodeId: state.region?.id,
        regionBounds: state.region?.bounds,
        fileKey: defaultFileKey
      });
      state.setSent(true);
      setTimeout(() => state.setSent(false), 2500);
    } catch (err) {
      state.setError(err instanceof Error ? err.message : String(err));
    } finally {
      state.setSending(false);
    }
  }

  // Reactivates a past session from the history ribbon — mirrors
  // fetchFromFigma's state updates, but doesn't push back into history
  // (reactivating isn't "loading new", see the module comment above). The
  // hit-map isn't part of a history entry (only screenshot + metadata are
  // persisted), so if it was linked to a Figma node, re-fetch the subtree
  // live — same reasoning as fetchFromFigma, so pin-a-region still works
  // after reactivating.
  function loadFromHistory(entry: CompareHistoryEntryData) {
    if (entry.figmaW && entry.figmaH) state.setFigmaSize({ w: entry.figmaW, h: entry.figmaH });
    state.setNodeId(entry.nodeId);
    state.setHitTree(null);
    state.setRootBounds(null);
    state.setRegion(null);
    if (entry.fetchedNodeId) {
      state.setFetchedNodeId(entry.fetchedNodeId);
      getFigmaSubtree(entry.fetchedNodeId, defaultFileKey)
        .then((subtree) => {
          state.setRootBounds(subtree.bounds);
          state.setHitTree(subtree);
        })
        .catch(() => {
          // Best-effort — the comparison itself still loads fine without a
          // hit-map, pin-a-region just won't be available for this session.
        });
    } else {
      state.setFetchedNodeId(null);
    }
    loadImage(entry.image);
  }

  return {
    loadImage,
    cycleDiffMode,
    resetView,
    onPaste,
    pasteFromClipboard,
    onDrop,
    fetchFromFigma,
    fetchFigmaSelection,
    resolveClickToRegion,
    matchZoom,
    sendToAgent,
    loadFromHistory
  };
}

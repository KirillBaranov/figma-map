import type { HitBounds } from "../lib/hitmap";
import type { Pos } from "./hooks/useDraggable";
import type { RegionPin, Size } from "./hooks/useCompareState";

interface CompareImageProps {
  image: string | null;
  naturalSize: Size | null;
  pos: Pos;
  scale: number;
  opacity: number;
  hidden: boolean;
  diffMode: boolean;
  diffContrast: boolean;
  diffSrc: string | null;
  syncScroll: boolean;
  dragging: boolean;
  onImageMouseDown: (e: React.MouseEvent) => void;
  region: RegionPin | null;
  rootBounds: HitBounds | null;
}

// The overlay's draggable reference image plus the pinned-region highlight
// — the visual layer of "overlay compare", with no state or data-fetching
// of its own. Diff mode has two renderers: "Blend" (live CSS
// mix-blend-mode, the default) and "Contrast" (diffContrast on) — a
// pre-rendered false-color diff supplied by useDiffSnapshot. Until the
// first contrast snapshot lands, this falls back to Blend so there's no
// blank flash.
export function CompareImage({
  image,
  naturalSize,
  pos,
  scale,
  opacity,
  hidden,
  diffMode,
  diffContrast,
  diffSrc,
  syncScroll,
  dragging,
  onImageMouseDown,
  region,
  rootBounds
}: CompareImageProps) {
  const showDiffSnapshot = diffMode && diffContrast && !!diffSrc;
  return (
    <>
      {image && (
        <img
          src={showDiffSnapshot ? diffSrc! : image}
          draggable={false}
          onMouseDown={onImageMouseDown}
          className={`fm-compare-image ${dragging ? "fm-compare-dragging" : ""}`}
          style={{
            position: syncScroll ? "absolute" : "fixed",
            left: pos.x,
            top: pos.y,
            width: naturalSize ? (naturalSize.w * scale) / 100 : undefined,
            height: naturalSize ? (naturalSize.h * scale) / 100 : undefined,
            opacity: hidden ? 0 : opacity / 100,
            mixBlendMode: diffMode && !showDiffSnapshot ? "difference" : "normal"
          }}
        />
      )}
      {region && rootBounds && (
        <div
          className="fm-compare-region"
          style={{
            position: syncScroll ? "absolute" : "fixed",
            pointerEvents: "none",
            left: pos.x + (region.bounds.x - rootBounds.x) * (scale / 100),
            top: pos.y + (region.bounds.y - rootBounds.y) * (scale / 100),
            width: region.bounds.width * (scale / 100),
            height: region.bounds.height * (scale / 100),
            outline: "2px solid #00e0ff",
            outlineOffset: -1
          }}
        />
      )}
    </>
  );
}

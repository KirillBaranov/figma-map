import { useEffect, useRef, useState } from "react";
import { captureViewport } from "../../lib/figma";
import { computeAmplifiedDiff } from "../../lib/diffSnapshot";
import type { CompareState } from "./useCompareState";

// Backs diffContrast ("Contrast" diff mode in CompareImage.tsx) — the
// alternative to the live CSS mix-blend-mode: "difference" ("Blend" mode).
// Blend mode is cheap and live but two near-identical *dark* pixels
// difference to near-black too, hiding real misalignment on dark UI. This
// captures a screenshot instead and diffs it pixel-by-pixel, false-colored
// the same way internal/render/pixeldiff.go does for `verify pixeldiff` (red
// = differs, dimmed gray = matches) — legible regardless of how dark the
// page is, at the cost of being a snapshot rather than a live blend, and of
// flagging antialiasing noise on text (Figma's rasterizer and the browser's
// don't hint glyphs identically) as if it were misalignment.
//
// Recomputed once when contrast mode turns on, and again whenever pos/scale
// settle after a drag (debounced — chrome.tabs.captureVisibleTab is
// rate-limited, so this can't run on every drag frame). The diff freezes
// until the overlay's next settle or a manual refresh.
export function useDiffSnapshot(state: CompareState) {
  const [diffSrc, setDiffSrc] = useState<string | null>(null);
  const [computing, setComputing] = useState(false);
  const prevUrlRef = useRef<string | null>(null);
  const tokenRef = useRef(0);
  // tokenRef alone only guards against a newer recompute superseding an
  // older one — it does nothing if the component unmounts entirely (e.g.
  // the compare window closes, or the site gets disabled) mid-capture.
  // chrome.tabs.captureVisibleTab still resolves after that, and calling
  // setDiffSrc/setComputing on an unmounted component is exactly the kind
  // of dangling async work that reads as "the UI hangs" when it lands.
  const isMountedRef = useRef(true);
  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  async function recompute() {
    if (!state.diffMode || !state.diffContrast || !state.image || !state.naturalSize) return;
    const width = (state.naturalSize.w * state.scale) / 100;
    const height = (state.naturalSize.h * state.scale) / 100;
    // syncScroll means the overlay is `position: absolute` (document-anchored,
    // scrolls with the page) — state.pos is document-relative there, so it
    // has to be converted back to viewport-relative before asking the
    // background worker to screenshot it (captureVisibleTab only sees what's
    // currently on screen).
    const left = state.syncScroll ? state.pos.x - window.scrollX : state.pos.x;
    const top = state.syncScroll ? state.pos.y - window.scrollY : state.pos.y;

    const x = Math.max(0, left);
    const y = Math.max(0, top);
    const right = Math.min(window.innerWidth, left + width);
    const bottom = Math.min(window.innerHeight, top + height);
    if (right <= x || bottom <= y) return; // overlay is currently off-screen

    const token = ++tokenRef.current;
    setComputing(true);
    try {
      const dpr = window.devicePixelRatio || 1;
      const screenshotUrl = await captureViewport({ x, y, width: right - x, height: bottom - y }, dpr);
      const diffUrl = await computeAmplifiedDiff(state.image, screenshotUrl);
      if (token !== tokenRef.current || !isMountedRef.current) {
        URL.revokeObjectURL(diffUrl); // a newer recompute already won, or we're gone
        return;
      }
      if (prevUrlRef.current) URL.revokeObjectURL(prevUrlRef.current);
      prevUrlRef.current = diffUrl;
      setDiffSrc(diffUrl);
    } catch {
      // Best-effort — CompareImage falls back to the live CSS blend when
      // diffSrc is null, so a failed capture just means one stale frame.
    } finally {
      if (token === tokenRef.current && isMountedRef.current) setComputing(false);
    }
  }

  useEffect(() => {
    if (!state.diffMode || !state.diffContrast) {
      if (prevUrlRef.current) {
        URL.revokeObjectURL(prevUrlRef.current);
        prevUrlRef.current = null;
      }
      setDiffSrc(null);
      return;
    }
    const id = setTimeout(recompute, 350);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.diffMode, state.diffContrast, state.pos.x, state.pos.y, state.scale, state.image, state.syncScroll]);

  useEffect(
    () => () => {
      if (prevUrlRef.current) URL.revokeObjectURL(prevUrlRef.current);
    },
    []
  );

  return { diffSrc, computing, recompute };
}

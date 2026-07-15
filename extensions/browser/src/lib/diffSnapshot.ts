import { dataUrlToBitmap } from "./image";

// Same false-color scheme as internal/render/pixeldiff.go's PixelDiff: red
// where pixels differ beyond colorTol (brighter alpha = bigger delta), dimmed
// gray where they match. Contrast stays legible regardless of how dark the
// underlying UI is — unlike CSS mix-blend-mode: "difference", where two
// near-identical dark pixels difference to near-black too, hiding a real
// misalignment until you crank opacity by hand.
//
// Higher than the Go CLI's default of 10 (internal/render/pixeldiff.go):
// that default compares two screenshots taken the same way at matched DPI,
// while this diffs a live browser render against a reference image that's
// been through an extra resample (drawImage stretching it to the captured
// crop's device-pixel size) — text edges pick up antialiasing differences
// well above 10/255 even when perfectly aligned. 10 lit up every glyph edge
// as "different" and buried the real misalignment signal in that noise.
const DEFAULT_COLOR_TOL = 32;

export async function computeAmplifiedDiff(refDataUrl: string, gotDataUrl: string, colorTol = DEFAULT_COLOR_TOL): Promise<string> {
  const [refBmp, gotBmp] = await Promise.all([dataUrlToBitmap(refDataUrl), dataUrlToBitmap(gotDataUrl)]);

  const w = gotBmp.width;
  const h = gotBmp.height;

  const refCanvas = new OffscreenCanvas(w, h);
  const refCtx = refCanvas.getContext("2d")!;
  refCtx.drawImage(refBmp, 0, 0, w, h);
  const refData = refCtx.getImageData(0, 0, w, h).data;

  const gotCanvas = new OffscreenCanvas(w, h);
  const gotCtx = gotCanvas.getContext("2d")!;
  gotCtx.drawImage(gotBmp, 0, 0, w, h);
  const gotData = gotCtx.getImageData(0, 0, w, h).data;

  const out = new ImageData(w, h);
  const outData = out.data;

  for (let i = 0; i < refData.length; i += 4) {
    const dr = Math.abs(refData[i] - gotData[i]);
    const dg = Math.abs(refData[i + 1] - gotData[i + 1]);
    const db = Math.abs(refData[i + 2] - gotData[i + 2]);
    const maxCh = Math.max(dr, dg, db);

    if (maxCh > colorTol) {
      outData[i] = 255;
      outData[i + 1] = 0;
      outData[i + 2] = 0;
      outData[i + 3] = Math.min(255, maxCh * 2);
    } else {
      outData[i] = refData[i] >> 1;
      outData[i + 1] = refData[i + 1] >> 1;
      outData[i + 2] = refData[i + 2] >> 1;
      outData[i + 3] = 128;
    }
  }

  const outCanvas = new OffscreenCanvas(w, h);
  const outCtx = outCanvas.getContext("2d")!;
  outCtx.putImageData(out, 0, 0);
  const blob = await outCanvas.convertToBlob({ type: "image/png" });
  return URL.createObjectURL(blob);
}

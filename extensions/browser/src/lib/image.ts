export async function dataUrlToBitmap(dataUrl: string): Promise<ImageBitmap> {
  const blob = await (await fetch(dataUrl)).blob();
  return createImageBitmap(blob);
}

export async function blobToBase64(blob: Blob): Promise<string> {
  const buf = await blob.arrayBuffer();
  let binary = "";
  const bytes = new Uint8Array(buf);
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
  return btoa(binary);
}

export async function cropBitmapToBase64(
  bitmap: ImageBitmap,
  sx: number,
  sy: number,
  sw: number,
  sh: number
): Promise<string> {
  const w = Math.max(1, Math.round(sw));
  const h = Math.max(1, Math.round(sh));
  const canvas = new OffscreenCanvas(w, h);
  const ctx = canvas.getContext("2d")!;
  ctx.drawImage(bitmap, sx, sy, sw, sh, 0, 0, w, h);
  const blob = await canvas.convertToBlob({ type: "image/png" });
  return blobToBase64(blob);
}

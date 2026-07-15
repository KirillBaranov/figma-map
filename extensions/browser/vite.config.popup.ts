import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// No viteSingleFile here: this HTML is loaded as a real extension page
// (action.default_popup), subject to MV3's non-negotiable extension_pages
// CSP (script-src 'self' — inline <script> is rejected outright, no
// manifest override can loosen it). Inlining the bundle into an inline
// <script> tag silently breaks the popup with a CSP violation instead of
// rendering. External module scripts referenced via <script src="...">
// are same-origin (chrome-extension://) and load fine.
export default defineConfig({
  plugins: [react()],
  root: "./src/popup",
  // manifest.json points at "dist/popup/index.html" — nested under the
  // extension root, not served from it — so asset URLs must be relative
  // (index.html's own directory), not root-absolute (default base: "/"
  // would resolve to chrome-extension://<id>/assets/... instead of
  // chrome-extension://<id>/dist/popup/assets/..., a 404).
  base: "./",
  build: {
    target: "es2020",
    cssCodeSplit: false,
    outDir: "../../dist/popup",
    emptyOutDir: true
  }
});

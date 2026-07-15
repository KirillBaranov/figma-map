import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// No viteSingleFile — same reasoning as vite.config.popup.ts: this HTML is
// a real extension page (options_page), and MV3's extension_pages CSP
// rejects inline <script> unconditionally, which is exactly what inlining
// the bundle produces.
export default defineConfig({
  plugins: [react()],
  root: "./src/options",
  // Same reasoning as vite.config.popup.ts's base — manifest.json points at
  // "dist/options/index.html", nested under the extension root.
  base: "./",
  build: {
    target: "es2020",
    cssCodeSplit: false,
    outDir: "../../dist/options",
    emptyOutDir: true
  }
});

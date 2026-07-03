import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Content scripts can't load separate chunks/CSS files via <link>/import maps,
// so this bundles React + the kit + the overlay CSS (inlined as a string,
// see content/index.tsx) into one IIFE — the same approach extensions/plugin uses
// for its main-thread bundle (vite.config.main.ts).
export default defineConfig({
  plugins: [react()],
  // build.lib mode (unlike the default app build) doesn't auto-replace
  // process.env.NODE_ENV, but react-dom reads it directly — without this
  // the bundle throws "process is not defined" as soon as it runs on a page.
  define: {
    "process.env.NODE_ENV": JSON.stringify("production")
  },
  build: {
    target: "es2020",
    lib: {
      entry: "src/content/index.tsx",
      formats: ["iife"],
      name: "figmaMapContent",
      fileName: () => "content.js"
    },
    outDir: "dist",
    emptyOutDir: false,
    minify: "esbuild"
  }
});

import { defineConfig } from "vite";

export default defineConfig({
  build: {
    target: "es2020",
    lib: {
      entry: "src/background.ts",
      formats: ["iife"],
      name: "figmaMapBackground",
      fileName: () => "background.js"
    },
    outDir: "dist",
    emptyOutDir: false,
    minify: false
  }
});

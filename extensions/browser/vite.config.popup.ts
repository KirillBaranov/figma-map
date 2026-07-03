import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { viteSingleFile } from "vite-plugin-singlefile";

export default defineConfig({
  plugins: [react(), viteSingleFile()],
  root: "./src/popup",
  build: {
    target: "es2020",
    cssCodeSplit: false,
    outDir: "../../dist/popup",
    rollupOptions: {
      output: { inlineDynamicImports: true }
    },
    emptyOutDir: true
  }
});

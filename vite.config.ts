import react from "@vitejs/plugin-react";
import path from "node:path";
import { defineConfig } from "vite";

const rootDir = import.meta.dirname;

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(rootDir, "src"),
    },
    dedupe: ["react", "react-dom"],
  },
  build: {
    chunkSizeWarningLimit: 5120,
  },
  server: {
    host: "0.0.0.0",
    port: 40249,
    strictPort: true,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:40248",
        changeOrigin: true,
      },
      "^/.*/-/": {
        target: "http://127.0.0.1:40248",
        changeOrigin: true,
      },
    },
  },
});

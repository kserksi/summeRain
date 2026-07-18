import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { sri } from "vite-plugin-sri3";
import fs from "node:fs";
import path from "node:path";

const hasCert = fs.existsSync(path.resolve(import.meta.dirname, "localhost+1-key.pem"));
const devBackendURL = process.env.VITE_DEV_BACKEND_URL ?? "http://localhost:8080";
const crossOriginIsolation =
  (process.env.VITE_CROSS_ORIGIN_ISOLATION ?? process.env.CROSS_ORIGIN_ISOLATION ?? "true") !==
  "false";
const crossOriginIsolationHeaders = {
  "Cross-Origin-Opener-Policy": "same-origin",
  "Cross-Origin-Embedder-Policy": "require-corp",
};

export default defineConfig({
  base: "/",
  plugins: [react(), tailwindcss(), sri()],
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "./src") },
  },
  server: {
    headers: crossOriginIsolation ? crossOriginIsolationHeaders : undefined,
    https: hasCert
      ? {
          key: fs.readFileSync(path.resolve(import.meta.dirname, "localhost+1-key.pem")),
          cert: fs.readFileSync(path.resolve(import.meta.dirname, "localhost+1.pem")),
        }
      : undefined,
    proxy: {
      "/api/": {
        target: devBackendURL,
        changeOrigin: false,
        xfwd: true,
        secure: false,
        timeout: 30000,
      },
      "/i/": {
        target: devBackendURL,
        changeOrigin: true,
        secure: false,
        timeout: 60000,
      },
    },
  },
  build: {
    // The observed Android fleet includes Chrome 93; Vite's moving default
    // target is newer and must not silently raise the browser floor.
    target: "es2020",
    outDir: "../backend/web",
    emptyOutDir: true,
  },
});

import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  // The admin panel lives at the root of https://vpnadmin.mydayai.uz.
  // No URL prefix means assets resolve straight from /assets/, keeping
  // the index.html and asset tree portable if the subdomain ever
  // changes again.
  base: "/",
  build: {
    outDir: "dist",
    sourcemap: false,
  },
  server: {
    port: 5173,
    // Local dev proxies /api/v1 to the production API by default so
    // the dev server can iterate against real data. Override with
    // VITE_API_URL when running a local backend — the axios client
    // picks that up at build time, bypassing the proxy entirely.
    proxy: {
      "/api/v1": {
        target: "https://vpnapi.mydayai.uz:9443",
        changeOrigin: true,
        secure: true,
      },
    },
  },
});

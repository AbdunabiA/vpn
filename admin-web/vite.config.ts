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
  // The admin panel is deployed under https://vpnapi.mydayai.uz:9443/admin/
  // so all static asset paths must be rewritten with that prefix. Vite's
  // `base` flag handles both the emitted HTML and the manifest.
  base: "/admin/",
  build: {
    outDir: "dist",
    sourcemap: false,
  },
  server: {
    port: 5173,
    // Local dev proxies /api/v1 to the production API by default so you
    // can iterate against real data; override with VITE_API_URL when you
    // want to point at a local backend.
    proxy: {
      "/api/v1": {
        target: "https://vpnapi.mydayai.uz:9443",
        changeOrigin: true,
        secure: true,
      },
    },
  },
});

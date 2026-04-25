import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Port 5830 is chosen to avoid collisions with commonly-used dev-tool defaults
// (Vite 5173, Next 3000, Django 8000, Tomcat 8080, Flask 5000, Postgres 5432,
// Redis 6379, Grafana 3000, Prometheus 9090, etc.). OSS installs that run many
// local tools side-by-side are the target audience.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5830,
    strictPort: true, // fail loudly if 5830 is taken rather than silently drifting
    open: false,
    proxy: {
      // Forward /api/* to the merged pindoc-server daemon (the same
      // process now serves /mcp/p/{project}, /api/..., and /health on a
      // single port — see cmd/pindoc-server). Keeps the UI on a single
      // origin so no CORS dance during dev. Vite's strictPort=true means
      // dev frontend and the daemon can't co-bind 5830 — switch the
      // daemon off (Stop-Service pindoc-server) before running
      // `pnpm dev` for hot-reload UI work.
      "/api": {
        target: "http://127.0.0.1:5830",
        changeOrigin: false,
      },
    },
  },
  preview: {
    port: 5830,
    strictPort: true,
  },
});

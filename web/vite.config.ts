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
  },
  preview: {
    port: 5830,
    strictPort: true,
  },
});

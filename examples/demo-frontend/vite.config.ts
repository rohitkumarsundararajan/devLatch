import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

// The demo frontend calls /api/* on its own origin; Vite forwards that to the
// backend. Point DEVLATCH_API_TARGET at the DevLatch proxy (e.g. :9000) to
// observe traffic; the default is the demo backend directly.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const target = env.DEVLATCH_API_TARGET || "http://127.0.0.1:8000";
  return {
    plugins: [react()],
    server: { port: 5173, proxy: { "/api": { target, changeOrigin: true } } },
  };
});

import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build lands in internal/webui/dist so Go can embed it.
export default defineConfig({
  plugins: [react()],
  base: "./",
  build: { outDir: "../internal/webui/dist", emptyOutDir: true },
  server: { proxy: { "/api": "http://127.0.0.1:7070" } },
});
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";
export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  build: { outDir: "dist", emptyOutDir: true },
  server: { proxy: {
    "/api": "http://127.0.0.1:20128",
    "/v1": "http://127.0.0.1:20128",
    "/healthz": "http://127.0.0.1:20128",
    "/readyz": "http://127.0.0.1:20128"
  }}
});

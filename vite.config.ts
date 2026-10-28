import { fileURLToPath, URL } from "node:url";

import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [vue()],
  root: fileURLToPath(new URL("./src/web", import.meta.url)),
  build: {
    outDir: fileURLToPath(new URL("./internal/webassets/dist", import.meta.url)),
    emptyOutDir: false
  },
  server: {
    proxy: { "/api": "http://127.0.0.1:3000" }
  }
});

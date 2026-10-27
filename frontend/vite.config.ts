import { defineConfig } from "vite";

// The built files go to dist, which Go embeds (frontend/embed.go) and which is committed so the
// desktop app builds without Node.
export default defineConfig({
  root: "src",
  base: "./",
  build: { outDir: "../dist", emptyOutDir: true, target: "es2020", sourcemap: false },
});

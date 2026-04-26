import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  root: "apps/web",
  plugins: [react()],
  build: {
    outDir: "../../dist/apps/web",
    emptyOutDir: true
  },
  server: {
    proxy: {
      "/api": {
        target: "http://127.0.0.1:3888",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, "")
      }
    }
  },
  test: {
    environment: "jsdom",
    setupFiles: "./vitest.setup.ts",
    coverage: {
      provider: "v8",
      reporter: ["text", "html"],
      include: ["src/core/**/*.ts"],
      exclude: ["src/core/types.ts", "src/core/index.ts"],
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 75,
        statements: 80
      }
    }
  }
});

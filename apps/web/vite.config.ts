import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const pagePilotTargetProxy = process.env.OMEGA_PAGE_PILOT_TARGET_URL ?? "http://127.0.0.1:5173";

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
      },
      "/page-pilot-target": {
        target: pagePilotTargetProxy,
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/page-pilot-target/, "") || "/"
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

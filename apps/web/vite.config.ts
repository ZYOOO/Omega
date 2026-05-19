import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const pagePilotTargetProxy = process.env.OMEGA_PAGE_PILOT_TARGET_URL ?? "http://127.0.0.1:3009";
const testSuite = process.env.OMEGA_TEST_SUITE ?? "current";
const currentTestInclude = [
  "src/__tests__/**/*.test.{ts,tsx}",
  "src/components/__tests__/**/*.test.{ts,tsx}"
];
const legacyTestInclude = [
  "src/core/__tests__/**/*.test.ts",
  "src/integrations/__tests__/**/*.test.ts",
  "src/local/__tests__/**/*.test.ts"
];
const testInclude =
  testSuite === "legacy"
    ? legacyTestInclude
    : testSuite === "all"
      ? [...currentTestInclude, ...legacyTestInclude]
      : currentTestInclude;

export default defineConfig({
  root: "apps/web",
  base: "./",
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
    include: testInclude,
    setupFiles: "./vitest.setup.ts",
    testTimeout: 30000,
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

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
import path from "node:path";
import { createRequire } from "node:module";
import { describe, expect, it } from "vitest";

const require = createRequire(import.meta.url);
const {
  buildPreviewRuntimePlan,
  buildPreviewRuntimeProfile,
  buildWebLaunchPlan,
  detectPackageManager,
  envFlag,
  selectPreviewRefreshStrategy,
  resolveRepositoryPreviewTarget,
  resolveRepositoryWorktree
} = require("../../../../apps/desktop/src/process-supervisor.cjs") as {
  buildPreviewRuntimePlan: (input: { env: Record<string, string | undefined> }) => {
    enabled: boolean;
    reason?: string;
    repoPath?: string;
    command?: string;
    args?: string[];
    shell?: boolean;
    source?: string;
    previewUrl: string;
  };
  buildPreviewRuntimeProfile: (input: { env: Record<string, string | undefined>; repoPath?: string; repositoryTargetId?: string; intent?: string }) => {
    plan: { enabled: boolean; command?: string; args?: string[]; source?: string; previewUrl: string };
    profile: {
      agentId: string;
      stageId: string;
      repositoryTargetId: string;
      workingDirectory: string;
      devCommand: string;
      previewUrl: string;
      source: string;
      evidence: string[];
      responsibilities: string[];
    };
  };
  buildWebLaunchPlan: (app: { isPackaged?: boolean }, env: Record<string, string | undefined>) => {
    mode: string;
    url: string;
    args?: string[];
  };
  detectPackageManager: (repoPath: string) => string;
  envFlag: (value?: string, defaultValue?: boolean) => boolean;
  selectPreviewRefreshStrategy: (profile: Record<string, unknown>, options?: { changedFiles?: string[] }) => string;
  resolveRepositoryPreviewTarget: (target: Record<string, unknown>, env: Record<string, string | undefined>) => Promise<{
    ok: boolean;
    repoPath?: string;
    htmlFile?: string;
    previewUrl?: string;
  }>;
  resolveRepositoryWorktree: (target: Record<string, unknown>, env: Record<string, string | undefined>) => Promise<string>;
};

describe("desktop process supervisor", () => {
  it("requires an explicit preview repository path", () => {
    const plan = buildPreviewRuntimePlan({ env: {} });

    expect(plan.enabled).toBe(false);
    expect(plan.reason).toContain("OMEGA_PREVIEW_REPO_PATH");
  });

  it("detects a Vite preview command from package.json", () => {
    const repo = mkdtempSync(path.join(tmpdir(), "omega-preview-"));
    try {
      writeFileSync(path.join(repo, "pnpm-lock.yaml"), "lockfileVersion: 9\n");
      writeFileSync(path.join(repo, "package.json"), JSON.stringify({ scripts: { dev: "vite --host 0.0.0.0" } }));

      const plan = buildPreviewRuntimePlan({
        env: {
          OMEGA_PREVIEW_REPO_PATH: repo,
          OMEGA_PREVIEW_URL: "http://127.0.0.1:6199/"
        }
      });

      expect(plan.enabled).toBe(true);
      expect(plan.command).toBe("pnpm");
      expect(plan.args).toEqual(["run", "dev", "--", "--host", "127.0.0.1", "--port", "6199"]);
      expect(plan.source).toBe("pnpm:dev");
    } finally {
      rmSync(repo, { recursive: true, force: true });
    }
  });

  it("prefers explicit preview commands over auto detection", () => {
    const repo = mkdtempSync(path.join(tmpdir(), "omega-preview-"));
    try {
      writeFileSync(path.join(repo, "package.json"), JSON.stringify({ scripts: { dev: "vite" } }));

      const plan = buildPreviewRuntimePlan({
        env: {
          OMEGA_PREVIEW_REPO_PATH: repo,
          OMEGA_PREVIEW_COMMAND: "npm run custom-preview",
          OMEGA_PREVIEW_URL: "http://127.0.0.1:6200/"
        }
      });

      expect(plan.enabled).toBe(true);
      expect(plan.command).toBe("npm run custom-preview");
      expect(plan.shell).toBe(true);
      expect(plan.source).toBe("env-command");
    } finally {
      rmSync(repo, { recursive: true, force: true });
    }
  });

  it("builds an auditable Preview Runtime Agent profile", () => {
    const repo = mkdtempSync(path.join(tmpdir(), "omega-preview-agent-"));
    try {
      writeFileSync(path.join(repo, "package.json"), JSON.stringify({ scripts: { dev: "vite" } }));
      writeFileSync(path.join(repo, "vite.config.ts"), "export default {}\n");

      const result = buildPreviewRuntimeProfile({
        repoPath: repo,
        repositoryTargetId: "repo_local",
        intent: "/settings",
        env: {
          OMEGA_PREVIEW_URL: "http://127.0.0.1:6222/",
        },
      });

      expect(result.plan.enabled).toBe(true);
      expect(result.profile).toMatchObject({
        agentId: "preview-runtime-agent",
        stageId: "preview_runtime",
        repositoryTargetId: "repo_local",
        workingDirectory: repo,
        previewUrl: "http://127.0.0.1:6222/",
        source: "npm:dev",
      });
      expect(result.profile.devCommand).toContain("npm run dev");
      expect(result.profile.evidence).toContain("package.json");
      expect(result.profile.evidence).toContain("vite.config.ts");
      expect(result.profile.responsibilities.join(" ")).toContain("selected repository workspace");
    } finally {
      rmSync(repo, { recursive: true, force: true });
    }
  });

  it("selects refresh or restart from the Preview Runtime Profile and changed files", () => {
    expect(selectPreviewRefreshStrategy({ source: "npm:dev", reloadStrategy: "hmr-wait" }, { changedFiles: ["src/App.tsx"] })).toBe("hmr-wait");
    expect(selectPreviewRefreshStrategy({ source: "npm:dev", reloadStrategy: "hmr-wait" }, { changedFiles: ["package.json"] })).toBe("server-restart");
    expect(selectPreviewRefreshStrategy({ source: "static-index" }, { changedFiles: ["index.html"] })).toBe("browser-reload");
    expect(selectPreviewRefreshStrategy({ source: "npm:dev", reloadStrategy: "hmr-wait" }, { changedFiles: ["vite.config.ts"] })).toBe("server-restart");
  });

  it("detects package managers by lockfile", () => {
    const repo = mkdtempSync(path.join(tmpdir(), "omega-preview-"));
    try {
      writeFileSync(path.join(repo, "yarn.lock"), "");
      expect(detectPackageManager(repo)).toBe("yarn");
    } finally {
      rmSync(repo, { recursive: true, force: true });
    }
  });

  it("parses desktop autostart flags", () => {
    expect(envFlag(undefined, true)).toBe(true);
    expect(envFlag("0", true)).toBe(false);
    expect(envFlag("false", true)).toBe(false);
    expect(envFlag("1", false)).toBe(true);
  });

  it("keeps the desktop web URL and Vite port aligned", () => {
    const plan = buildWebLaunchPlan({ isPackaged: false }, {});

    expect(plan.url).toBe("http://127.0.0.1:5173/");
    expect(plan.args).toContain("5173");
  });

  it("resolves a GitHub target from its isolated preview workspace", async () => {
    const root = mkdtempSync(path.join(tmpdir(), "omega-preview-workspaces-"));
    const repo = path.join(root, "ZYOOO_TestRepo");
    try {
      mkdirSync(repo, { recursive: true });
      execFileSync("git", ["init"], { cwd: repo });
      execFileSync("git", ["remote", "add", "origin", "git@github.com:ZYOOO/TestRepo.git"], { cwd: repo });
      writeFileSync(path.join(repo, "index.html"), "<main>Preview</main>\n");

      const target = { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" };
      await expect(resolveRepositoryWorktree(target, { OMEGA_PAGE_PILOT_WORKSPACE_ROOT: root })).resolves.toBe(repo);

      const preview = await resolveRepositoryPreviewTarget(target, {
        OMEGA_PAGE_PILOT_WORKSPACE_ROOT: root,
        OMEGA_PREVIEW_URL: "http://127.0.0.1:3009/",
      });
      expect(preview).toMatchObject({
        ok: true,
        repoPath: repo,
        htmlFile: path.join(repo, "index.html"),
        previewUrl: "http://127.0.0.1:3009/",
      });
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});

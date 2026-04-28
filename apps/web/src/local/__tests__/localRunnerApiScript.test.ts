import { mkdtemp, rm } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { spawn } from "child_process";
import { describe, expect, it } from "vitest";

function startScript(workspaceRoot: string): Promise<{
  child: ReturnType<typeof spawn>;
  url: string;
}> {
  return new Promise((resolve, reject) => {
    const child = spawn(
      process.execPath,
      ["scripts/local-runner-api.mjs", "--workspace-root", workspaceRoot, "--port", "0"],
      { cwd: process.cwd(), shell: false }
    );

    let stderr = "";
    if (!child.stderr || !child.stdout) {
      reject(new Error("local runner API script stdio streams are unavailable"));
      return;
    }
    child.stderr.on("data", (chunk: Buffer) => {
      stderr += chunk.toString();
    });
    child.stdout.on("data", (chunk: Buffer) => {
      const text = chunk.toString();
      const match = text.match(/Mission Control API listening: (http:\/\/[^\s]+)/);
      if (match) {
        resolve({ child, url: match[1] });
      }
    });
    child.on("close", (code) => {
      if (code !== null && code !== 0) {
        reject(new Error(stderr));
      }
    });
  });
}

describe("local-runner-api script", () => {
  it("starts a local Mission Control API server", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-api-script-"));
    const { child, url } = await startScript(workspaceRoot);

    try {
      const response = await fetch(`${url}/health`);
      expect(response.status).toBe(200);
      expect(await response.json()).toMatchObject({ ok: true, persistence: "sqlite" });
    } finally {
      child.kill("SIGTERM");
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });
});

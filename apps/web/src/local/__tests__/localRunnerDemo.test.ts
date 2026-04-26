import { mkdtemp, readFile, rm } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { spawn } from "child_process";
import { describe, expect, it } from "vitest";

function runDemo(workspaceRoot: string): Promise<{
  exitCode: number | null;
  stdout: string;
  stderr: string;
}> {
  return new Promise((resolve) => {
    const child = spawn(process.execPath, ["scripts/local-runner-demo.mjs", workspaceRoot], {
      cwd: process.cwd(),
      shell: false
    });

    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (chunk: Buffer) => {
      stdout += chunk.toString();
    });

    child.stderr.on("data", (chunk: Buffer) => {
      stderr += chunk.toString();
    });

    child.on("close", (exitCode) => resolve({ exitCode, stdout, stderr }));
  });
}

describe("local runner demo CLI", () => {
  it("runs a local Mission Control job and writes proof into a workspace", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-demo-"));

    try {
      const result = await runDemo(workspaceRoot);

      expect(result.exitCode).toBe(0);
      expect(result.stderr).toBe("");
      expect(result.stdout).toContain("Local runner demo passed");
      expect(result.stdout).toContain("OMG-LOCAL-testing");

      const proof = await readFile(
        join(workspaceRoot, "OMG-LOCAL-testing", ".omega", "proof", "local-check.txt"),
        "utf8"
      );
      expect(proof).toContain("local runner proof");
    } finally {
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });
});

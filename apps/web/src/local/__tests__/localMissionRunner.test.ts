import { mkdtemp, readFile, rm } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { describe, expect, it } from "vitest";
import { runLocalMissionJob } from "../localMissionRunner";

describe("runLocalMissionJob", () => {
  it("creates an isolated workspace, runs a local command, and captures proof", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-runner-"));

    try {
      const result = await runLocalMissionJob({
        runId: "run_1",
        issueKey: "OMG-1",
        stageId: "testing",
        agentId: "testing",
        workspaceRoot,
        prompt: "Write tests and report coverage.",
        command: {
          executable: process.execPath,
          args: [
            "-e",
            "const fs=require('fs'); fs.writeFileSync('.omega/proof/coverage.txt','coverage: 91%'); console.log('runner complete')"
          ]
        }
      });

      expect(result.status).toBe("passed");
      expect(result.workspacePath).toContain("OMG-1-testing");
      expect(result.stdout).toContain("runner complete");
      expect(result.proofFiles).toEqual(
        expect.arrayContaining([expect.stringContaining("coverage.txt")])
      );

      const jobSpec = await readFile(join(result.workspacePath, ".omega", "job.json"), "utf8");
      expect(JSON.parse(jobSpec)).toMatchObject({
        runId: "run_1",
        issueKey: "OMG-1",
        stageId: "testing",
        agentId: "testing"
      });
    } finally {
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });

  it("returns a failed result with stderr when the local command fails", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-runner-"));

    try {
      const result = await runLocalMissionJob({
        runId: "run_2",
        issueKey: "OMG-2",
        stageId: "review",
        agentId: "review",
        workspaceRoot,
        prompt: "Review the PR.",
        command: {
          executable: process.execPath,
          args: ["-e", "console.error('review failed'); process.exit(7)"]
        }
      });

      expect(result.status).toBe("failed");
      expect(result.exitCode).toBe(7);
      expect(result.stderr).toContain("review failed");
    } finally {
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });

  it("can pass a workspace prompt file to the command through stdin", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-runner-"));

    try {
      const result = await runLocalMissionJob({
        runId: "run_3",
        issueKey: "OMG-3",
        stageId: "intake",
        agentId: "requirement",
        workspaceRoot,
        prompt: "stdin prompt works",
        command: {
          executable: process.execPath,
          args: [
            "-e",
            "let data=''; process.stdin.on('data', c => data += c); process.stdin.on('end', () => { console.log(data.trim()) })"
          ],
          stdinFile: ".omega/prompt.md"
        }
      });

      expect(result.status).toBe("passed");
      expect(result.stdout).toContain("stdin prompt works");
    } finally {
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });
});

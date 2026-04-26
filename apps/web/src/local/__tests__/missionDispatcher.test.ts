import { mkdtemp, rm } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { describe, expect, it } from "vitest";
import { createMissionFromRun, createSampleRun, createWorkItems } from "../../core";
import { dispatchMissionOperation } from "../missionDispatcher";

describe("dispatchMissionOperation", () => {
  it("dispatches a mission operation through the local runner", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-dispatch-"));
    const run = createSampleRun();
    const item = createWorkItems(run)[0];
    const mission = createMissionFromRun(run, item);

    try {
      const result = await dispatchMissionOperation({
        mission,
        operation: mission.operations[0],
        workspaceRoot,
        command: {
          executable: process.execPath,
          args: [
            "-e",
            "const fs=require('fs'); fs.writeFileSync('.omega/proof/dispatch.txt','dispatch proof'); console.log('dispatch complete')"
          ]
        }
      });

      expect(result.status).toBe("passed");
      expect(result.workspacePath).toContain("OMG-1-intake");
      expect(result.stdout).toContain("dispatch complete");
      expect(result.proofFiles).toEqual(expect.arrayContaining([expect.stringContaining("dispatch.txt")]));
    } finally {
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });
});

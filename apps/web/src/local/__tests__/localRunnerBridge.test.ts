import { mkdtemp, rm } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { describe, expect, it } from "vitest";
import { createMissionFromRun, createSampleRun, createWorkItems } from "../../core";
import { LocalRunnerBridge } from "../localRunnerBridge";

describe("LocalRunnerBridge", () => {
  it("dispatches a mission operation and returns a bridge response", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-bridge-"));
    const run = createSampleRun();
    const mission = createMissionFromRun(run, createWorkItems(run)[0]);
    const bridge = new LocalRunnerBridge(workspaceRoot);

    try {
      const response = await bridge.runOperation({
        mission,
        operationId: "operation_intake",
        command: {
          executable: process.execPath,
          args: [
            "-e",
            "const fs=require('fs'); fs.writeFileSync('.omega/proof/bridge.txt','bridge proof'); console.log('bridge complete')"
          ]
        }
      });

      expect(response.status).toBe("passed");
      expect(response.operationId).toBe("operation_intake");
      expect(response.proofFiles).toEqual(expect.arrayContaining([expect.stringContaining("bridge.txt")]));
      expect(response.events.map((event) => event.type)).toEqual([
        "operation.started",
        "operation.proof-attached",
        "operation.completed"
      ]);
      expect(response.events[0]).toMatchObject({ workItemId: "item_intake" });
    } finally {
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });
});

import { describe, expect, it } from "vitest";
import {
  applyMissionControlEvents,
  createMissionControlState,
  createSampleRun,
  createWorkItems
} from "..";

describe("mission control state", () => {
  it("applies operation events to workboard items and event history", () => {
    const run = createSampleRun();
    const state = createMissionControlState(run, createWorkItems(run));
    const next = applyMissionControlEvents(state, [
      {
        type: "operation.started",
        missionId: "mission_1",
        workItemId: "item_intake",
        operationId: "operation_intake",
        operationTitle: "Intake",
        timestamp: "2026-04-22T00:00:00.000Z"
      },
      {
        type: "operation.proof-attached",
        missionId: "mission_1",
        workItemId: "item_intake",
        operationId: "operation_intake",
        operationTitle: "Intake",
        proofFiles: [".omega/proof/a.txt"],
        summary: "Proof attached.",
        timestamp: "2026-04-22T00:01:00.000Z"
      },
      {
        type: "operation.completed",
        missionId: "mission_1",
        workItemId: "item_intake",
        operationId: "operation_intake",
        operationTitle: "Intake",
        timestamp: "2026-04-22T00:02:00.000Z"
      }
    ]);

    expect(next.workItems[0].status).toBe("Done");
    expect(next.workItems[0].labels).toEqual(expect.arrayContaining(["proof-attached"]));
    expect(next.events).toHaveLength(3);
    expect(next.syncIntents).toHaveLength(5);
    expect(state.workItems[0].status).toBe("Ready");
  });
});

import { describe, expect, it } from "vitest";
import { createMissionFromRun, createSampleRun, createWorkItems } from "..";

describe("createMissionFromRun", () => {
  it("creates a mission from a delivery run and selected issue", () => {
    const run = createSampleRun();
    const item = createWorkItems(run)[0];
    const mission = createMissionFromRun(run, item);

    expect(mission).toMatchObject({
      id: "mission_OMEGA-1_intake",
      sourceIssueKey: "OMG-1",
      sourceWorkItemId: "item_intake",
      title: "Intake",
      status: "ready",
      checkpointRequired: true
    });
    expect(mission.operations).toHaveLength(1);
    expect(mission.operations[0]).toMatchObject({
      id: "operation_intake",
      agentId: "requirement",
      status: "ready"
    });
    expect(mission.operations[0].prompt).toContain("Source work item: OMG-1");
    expect(mission.operations[0].prompt).toContain("Acceptance criteria");
    expect(mission.links.map((link) => link.provider)).toEqual(["workboard", "github", "ci"]);
  });
});

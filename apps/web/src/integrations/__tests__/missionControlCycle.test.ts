import { describe, expect, it } from "vitest";
import { applyMissionControlEvents, createMissionControlState, createSampleRun, createWorkItems } from "../../core";
import { ConnectorRuntime } from "../connectorRuntime";
import { InMemoryGitHubClient } from "../githubClient";

describe("Mission Control connector cycle", () => {
  it("applies mission events and executes resulting connector intents", async () => {
    const run = createSampleRun();
    const workItems = createWorkItems(run);
    const state = createMissionControlState(run, workItems);
    const next = applyMissionControlEvents(state, [
      {
        type: "operation.proof-attached",
        missionId: "GH-1",
        workItemId: "item_intake",
        operationId: "operation_intake",
        operationTitle: "Intake",
        proofFiles: [".omega/proof/intake.txt"],
        summary: "Intake proof attached.",
        timestamp: "2026-04-22T00:00:00.000Z"
      }
    ]);

    const github = new InMemoryGitHubClient({
      issues: [{ id: 1, number: 1, title: "Intake", state: "open", labels: [], assignees: [] }],
      pullRequests: [],
      checkRuns: []
    });
    const runtime = new ConnectorRuntime({ workItems, github });
    const report = await runtime.execute(next.syncIntents);

    expect(report.failed).toHaveLength(0);
    expect(runtime.workItems[0].labels).toEqual(expect.arrayContaining(["proof-attached"]));
    expect(github.comments[0].body).toContain("Intake proof attached.");
  });
});

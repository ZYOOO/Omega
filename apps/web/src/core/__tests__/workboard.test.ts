import { describe, expect, it } from "vitest";
import {
  createSampleRun,
  createWorkItems,
  createWorkboardProject,
  groupWorkItemsByStatus,
  updateWorkItemPriority,
  updateWorkItemStatus
} from "..";

describe("workboard", () => {
  it("creates an Omega-owned project and work items from a delivery run", () => {
    const run = createSampleRun();
    const project = createWorkboardProject(run);
    const items = createWorkItems(run);

    expect(project).toMatchObject({
      id: "project_req_omega_001",
      name: "OMEGA-1",
      team: "Omega",
      status: "Active",
      repositoryTargets: []
    });
    expect(items[0]).toMatchObject({
      key: "OMG-1",
      title: "Intake",
      status: "Ready",
      priority: "High",
      team: "Omega",
      source: "ai_generated",
      acceptanceCriteria: ["Scope is clear", "Acceptance criteria are testable", "Open questions are recorded"],
      blockedByItemIds: []
    });
  });

  it("groups work items by status in workboard order", () => {
    const groups = groupWorkItemsByStatus(createWorkItems(createSampleRun()));

    expect(groups.map((group) => group.status)).toEqual(["Ready", "Backlog"]);
    expect(groups[0].items.map((item) => item.key)).toEqual(["OMG-1"]);
    expect(groups[1].items).toHaveLength(5);
  });

  it("keeps human review and blocked queues visible before done work", () => {
    const items = updateWorkItemStatus(
      updateWorkItemStatus(
        updateWorkItemStatus(createWorkItems(createSampleRun()), "item_solution", "Human Review"),
        "item_coding",
        "Done"
      ),
      "item_testing",
      "Blocked"
    );
    const groups = groupWorkItemsByStatus(items);

    expect(groups.map((group) => group.status)).toEqual(["Ready", "Human Review", "Backlog", "Blocked", "Done"]);
  });

  it("updates work item status and priority immutably", () => {
    const items = createWorkItems(createSampleRun());
    const updatedStatus = updateWorkItemStatus(items, "item_intake", "In Review");
    const updatedPriority = updateWorkItemPriority(updatedStatus, "item_intake", "Urgent");

    expect(items[0].status).toBe("Ready");
    expect(updatedStatus[0].status).toBe("In Review");
    expect(updatedPriority[0].priority).toBe("Urgent");
  });
});

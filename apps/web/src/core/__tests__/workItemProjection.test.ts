import { describe, expect, it } from "vitest";
import {
  createProjectedWorkItems,
  groupProjectedWorkItemsByStatus,
  updateProjectedWorkItemPriority,
  updateProjectedWorkItemStatus
} from "../workItemProjection";
import { createSampleRun } from "..";

describe("work item projection model", () => {
  it("creates delivery work items from pipeline stages with Omega keys and properties", () => {
    const items = createProjectedWorkItems(createSampleRun());

    expect(items).toHaveLength(6);
    expect(items[0]).toMatchObject({
      key: "OMG-1",
      title: "Intake",
      status: "Ready",
      priority: "High",
      team: "Omega"
    });
  });

  it("groups work items by status in workflow order", () => {
    const items = createProjectedWorkItems(createSampleRun());
    const groups = groupProjectedWorkItemsByStatus(items);

    expect(groups.map((group) => group.status)).toEqual(["Ready", "Backlog"]);
    expect(groups[0].items.map((item) => item.key)).toEqual(["OMG-1"]);
    expect(groups[1].items).toHaveLength(5);
  });

  it("updates work item status and priority immutably", () => {
    const items = createProjectedWorkItems(createSampleRun());
    const updatedStatus = updateProjectedWorkItemStatus(items, "item_intake", "In Review");
    const updatedPriority = updateProjectedWorkItemPriority(updatedStatus, "item_intake", "Urgent");

    expect(items[0].status).toBe("Ready");
    expect(updatedStatus[0].status).toBe("In Review");
    expect(updatedPriority[0].priority).toBe("Urgent");
  });
});

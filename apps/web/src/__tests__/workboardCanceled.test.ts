import { describe, expect, it } from "vitest";
import { createSampleRun, createWorkItems, groupWorkItemsByStatus, updateWorkItemStatus } from "../core";

describe("workboard canceled status", () => {
  it("keeps canceled work terminal without mixing it into blocked work", () => {
    const items = updateWorkItemStatus(
      updateWorkItemStatus(createWorkItems(createSampleRun()), "item_solution", "Canceled"),
      "item_testing",
      "Blocked"
    );
    const groups = groupWorkItemsByStatus(items);

    expect(groups.map((group) => group.status)).toEqual(["Ready", "Backlog", "Blocked", "Canceled"]);
    expect(groups.find((group) => group.status === "Canceled")?.items.map((item) => item.id)).toEqual(["item_solution"]);
  });
});

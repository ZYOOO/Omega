import { describe, expect, it } from "vitest";
import { createSampleRun, createWorkItems, createWorkboardView } from "..";

describe("createWorkboardView", () => {
  it("filters work items by status, assignee, and label", () => {
    const items = createWorkItems(createSampleRun());
    const view = createWorkboardView(items, {
      filters: {
        status: ["Backlog"],
        assignee: ["testing"],
        labels: ["human-gate"]
      },
      sort: { field: "priority", direction: "desc" },
      display: ["key", "title", "status", "priority"]
    });

    expect(view.items).toHaveLength(1);
    expect(view.items[0]).toMatchObject({
      key: "OMG-4",
      title: "Testing",
      status: "Backlog",
      priority: "High"
    });
  });

  it("sorts urgent work ahead of low priority work", () => {
    const items = [
      { ...createWorkItems(createSampleRun())[0], id: "low", key: "LOW", priority: "Low" as const },
      { ...createWorkItems(createSampleRun())[1], id: "urgent", key: "URG", priority: "Urgent" as const }
    ];

    const view = createWorkboardView(items, {
      filters: {},
      sort: { field: "priority", direction: "desc" },
      display: ["key", "priority"]
    });

    expect(view.items.map((item) => item.key)).toEqual(["URG", "LOW"]);
  });
});

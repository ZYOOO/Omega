import { describe, expect, it } from "vitest";
import { applyWorkboardSyncIntent, createSampleRun, createWorkItems } from "..";

describe("applyWorkboardSyncIntent", () => {
  it("updates a work item status from a sync intent", () => {
    const items = createWorkItems(createSampleRun());
    const updated = applyWorkboardSyncIntent(items, {
      provider: "workboard",
      action: "update-status",
      targetId: "item_intake",
      payload: { status: "In Review" }
    });

    expect(updated[0].status).toBe("In Review");
    expect(items[0].status).toBe("Ready");
  });

  it("attaches proof metadata to a work item label set", () => {
    const items = createWorkItems(createSampleRun());
    const updated = applyWorkboardSyncIntent(items, {
      provider: "workboard",
      action: "attach-proof",
      targetId: "item_intake",
      payload: {
        operationTitle: "Intake",
        proofFiles: [".omega/proof/a.txt"],
        summary: "Proof collected."
      }
    });

    expect(updated[0].labels).toEqual(expect.arrayContaining(["proof-attached"]));
  });
});
